package sdktests

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

const (
	// Schema of the DynamoDB table
	dynamoDbTablePartitionKey = "namespace"
	dynamoDbTableName         = "sdk-contract-tests"
	dynamoDbTableSortKey      = "key"
	dynamoDbVersionAttribute  = "version"
	dynamoDbItemJSONAttribute = "item"

	// We won't try to store items whose total size exceeds this. The DynamoDB documentation says
	// only "400KB", which probably means 400*1024, but to avoid any chance of trying to store a
	// too-large item we are rounding it down.
	dynamoDbMaxItemSize = 400000
)

type DynamoDBPersistentStore struct {
	dynamodb *dynamodb.DynamoDB
}

func (d *DynamoDBPersistentStore) DSN() string {
	return ""
}

func (d *DynamoDBPersistentStore) Type() servicedef.SDKConfigPersistentType {
	return servicedef.DynamoDB
}

func (d *DynamoDBPersistentStore) Reset() error {
	d.dynamodb.DeleteTable(&dynamodb.DeleteTableInput{TableName: aws.String(dynamoDbTableName)})
	d.dynamodb.CreateTable(&dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String(dynamoDbTablePartitionKey),
				AttributeType: aws.String("S"),
			},
			{
				AttributeName: aws.String(dynamoDbTableSortKey),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String(dynamoDbTablePartitionKey),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String(dynamoDbTableSortKey),
				KeyType:       aws.String("RANGE"),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
		TableName: aws.String(dynamoDbTableName),
	})

	return nil
}

func (d *DynamoDBPersistentStore) Get(prefix, key string) (o.Maybe[string], error) {
	result, err := d.dynamodb.GetItem(
		&dynamodb.GetItemInput{
			TableName: aws.String(dynamoDbTableName),
			Key: map[string]*dynamodb.AttributeValue{
				dynamoDbTablePartitionKey: {S: aws.String(addPrefix(prefix, key))},
				dynamoDbTableSortKey:      {S: aws.String(addPrefix(prefix, key))},
			},
		})

	if err != nil || result == nil {
		return o.None[string](), err
	} else if result.Item == nil {
		return o.None[string](), nil
	} else if key == PersistenceInitedKey {
		return o.Some(""), nil
	}

	if len(result.Item) != 1 {
		return o.None[string](), nil
	}

	return o.Some(*result.Item[dynamoDbItemJSONAttribute].S), nil
}

func (d *DynamoDBPersistentStore) GetMap(prefix, key string) (map[string]string, error) {
	query := &dynamodb.QueryInput{
		TableName:      aws.String(dynamoDbTableName),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]*dynamodb.Condition{
			dynamoDbTablePartitionKey: {
				ComparisonOperator: aws.String(dynamodb.ComparisonOperatorEq),
				AttributeValueList: []*dynamodb.AttributeValue{
					{S: aws.String(addPrefix(prefix, key))},
				},
			},
		},
	}

	results := map[string]string{}
	response, err := d.dynamodb.Query(query)
	if err != nil {
		return results, err
	}

	for _, item := range response.Items {
		itemKey := *item[dynamoDbTableSortKey].S
		results[itemKey] = *item[dynamoDbItemJSONAttribute].S
	}

	return results, nil
}

func (d *DynamoDBPersistentStore) WriteMap(prefix, key string, data map[string]string) error {
	unusedKeys := make(map[string]struct{})

	condition := dynamodb.Condition{
		ComparisonOperator: aws.String("EQ"),
		AttributeValueList: []*dynamodb.AttributeValue{{
			S: aws.String(addPrefix(prefix, key)),
		}},
	}

	// Read in all the old keys first
	query := &dynamodb.QueryInput{
		TableName:      aws.String(dynamoDbTableName),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]*dynamodb.Condition{
			dynamoDbTablePartitionKey: &condition,
		},
	}

	response, err := d.dynamodb.Query(query)
	if err != nil {
		return err
	}

	for _, item := range response.Items {
		itemKey := item[dynamoDbTableSortKey].String()
		unusedKeys[itemKey] = struct{}{}
	}

	requests := make([]*dynamodb.WriteRequest, 0)

	for k, v := range data {
		var versioned struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal([]byte(v), &versioned); err != nil {
			return err
		}
		requests = append(requests, &dynamodb.WriteRequest{
			PutRequest: &dynamodb.PutRequest{
				Item: map[string]*dynamodb.AttributeValue{
					dynamoDbTablePartitionKey: {S: aws.String(addPrefix(prefix, key))},
					dynamoDbTableSortKey:      {S: aws.String(k)},
					dynamoDbItemJSONAttribute: {S: aws.String(v)},
					dynamoDbVersionAttribute:  {N: aws.String(strconv.Itoa(versioned.Version))},
				},
			},
		})
		delete(unusedKeys, k)
	}

	for k := range unusedKeys {
		if k == PersistenceInitedKey {
			continue
		}
		delKey := map[string]*dynamodb.AttributeValue{
			dynamoDbTablePartitionKey: {S: aws.String(addPrefix(prefix, key))},
			dynamoDbTableSortKey:      {S: aws.String(k)},
		}
		requests = append(requests, &dynamodb.WriteRequest{
			DeleteRequest: &dynamodb.DeleteRequest{Key: delKey},
		})
	}

	// Now set the special key that we check in InitializedInternal()
	requests = append(requests, &dynamodb.WriteRequest{
		PutRequest: &dynamodb.PutRequest{Item: map[string]*dynamodb.AttributeValue{
			dynamoDbTablePartitionKey: {S: aws.String(addPrefix(prefix, PersistenceInitedKey))},
			dynamoDbTableSortKey:      {S: aws.String(PersistenceInitedKey)},
		}},
	})

	if err := batchWriteRequests(context.Background(), d.dynamodb, dynamoDbTableName, requests); err != nil {
		// COVERAGE: can't cause an error here in unit tests because we only get this far if the
		// DynamoDB client is successful on the initial query
		return fmt.Errorf("failed to write %d items(s) in batches: %s", len(requests), err)
	}

	return nil
}

// batchWriteRequests executes a list of write requests (PutItem or DeleteItem)
// in batches of 25, which is the maximum BatchWriteItem can handle.
func batchWriteRequests(
	context context.Context,
	client *dynamodb.DynamoDB,
	table string,
	requests []*dynamodb.WriteRequest,
) error {
	for len(requests) > 0 {
		batchSize := int(math.Min(float64(len(requests)), 25))
		batch := requests[:batchSize]
		requests = requests[batchSize:]

		_, err := client.BatchWriteItem(&dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]*dynamodb.WriteRequest{table: batch},
		})
		if err != nil {
			// COVERAGE: can't simulate this condition in unit tests because we will only get this
			// far if the initial query in Init() already succeeded, and we don't have the ability
			// to make DynamoDB fail *selectively* within a single test
			return err
		}
	}
	return nil
}

func addPrefix(prefix, value string) string {
	if prefix == "" {
		return value
	}

	return prefix + ":" + value
}

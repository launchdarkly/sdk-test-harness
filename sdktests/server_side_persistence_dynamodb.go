package sdktests

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/dynamodb"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

const (
	// Schema of the DynamoDB table
	dynamoDBTablePartitionKey = "namespace"
	dynamoDBTableName         = "sdk-contract-tests"
	dynamoDBTableSortKey      = "key"
	dynamoDBVersionAttribute  = "version"
	dynamoDBItemJSONAttribute = "item"
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
	_, err := d.dynamodb.DeleteTable(&dynamodb.DeleteTableInput{TableName: aws.String(dynamoDBTableName)})
	var aerr awserr.Error
	if errors.As(err, &aerr) {
		switch aerr.Code() {
		case dynamodb.ErrCodeResourceNotFoundException:
			// pass
		default:
			return err
		}
	}

	_, err = d.dynamodb.CreateTable(&dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String(dynamoDBTablePartitionKey),
				AttributeType: aws.String("S"),
			},
			{
				AttributeName: aws.String(dynamoDBTableSortKey),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String(dynamoDBTablePartitionKey),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String(dynamoDBTableSortKey),
				KeyType:       aws.String("RANGE"),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
		TableName: aws.String(dynamoDBTableName),
	})
	return err
}

func (d *DynamoDBPersistentStore) Get(prefix, key string) (o.Maybe[string], error) {
	result, err := d.dynamodb.GetItem(
		&dynamodb.GetItemInput{
			TableName: aws.String(dynamoDBTableName),
			Key: map[string]*dynamodb.AttributeValue{
				dynamoDBTablePartitionKey: {S: aws.String(addPrefix(prefix, key))},
				dynamoDBTableSortKey:      {S: aws.String(addPrefix(prefix, key))},
			},
		})

	//nolint:gocritic  // if is better than switch here
	if err != nil || result == nil {
		return o.None[string](), err
	} else if result.Item == nil {
		return o.None[string](), nil
	} else if key == persistenceInitedKey {
		return o.Some(""), nil
	}

	if len(result.Item) != 1 {
		return o.None[string](), nil
	}

	return o.Some(*result.Item[dynamoDBItemJSONAttribute].S), nil
}

func (d *DynamoDBPersistentStore) GetMap(prefix, key string) (map[string]string, error) {
	query := &dynamodb.QueryInput{
		TableName:      aws.String(dynamoDBTableName),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]*dynamodb.Condition{
			dynamoDBTablePartitionKey: {
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
		itemKey := *item[dynamoDBTableSortKey].S
		results[itemKey] = *item[dynamoDBItemJSONAttribute].S
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
		TableName:      aws.String(dynamoDBTableName),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]*dynamodb.Condition{
			dynamoDBTablePartitionKey: &condition,
		},
	}

	response, err := d.dynamodb.Query(query)
	if err != nil {
		return err
	}

	for _, item := range response.Items {
		itemKey := item[dynamoDBTableSortKey].String()
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
					dynamoDBTablePartitionKey: {S: aws.String(addPrefix(prefix, key))},
					dynamoDBTableSortKey:      {S: aws.String(k)},
					dynamoDBItemJSONAttribute: {S: aws.String(v)},
					dynamoDBVersionAttribute:  {N: aws.String(strconv.Itoa(versioned.Version))},
				},
			},
		})
		delete(unusedKeys, k)
	}

	for k := range unusedKeys {
		if k == persistenceInitedKey {
			continue
		}
		delKey := map[string]*dynamodb.AttributeValue{
			dynamoDBTablePartitionKey: {S: aws.String(addPrefix(prefix, key))},
			dynamoDBTableSortKey:      {S: aws.String(k)},
		}
		requests = append(requests, &dynamodb.WriteRequest{
			DeleteRequest: &dynamodb.DeleteRequest{Key: delKey},
		})
	}

	// Now set the special key that we check in InitializedInternal()
	requests = append(requests, &dynamodb.WriteRequest{
		PutRequest: &dynamodb.PutRequest{Item: map[string]*dynamodb.AttributeValue{
			dynamoDBTablePartitionKey: {S: aws.String(addPrefix(prefix, persistenceInitedKey))},
			dynamoDBTableSortKey:      {S: aws.String(persistenceInitedKey)},
		}},
	})

	if err := batchWriteRequests(d.dynamodb, dynamoDBTableName, requests); err != nil {
		// COVERAGE: can't cause an error here in unit tests because we only get this far if the
		// DynamoDB client is successful on the initial query
		return fmt.Errorf("failed to write %d items(s) in batches: %s", len(requests), err)
	}

	return nil
}

// batchWriteRequests executes a list of write requests (PutItem or DeleteItem)
// in batches of 25, which is the maximum BatchWriteItem can handle.
func batchWriteRequests(
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

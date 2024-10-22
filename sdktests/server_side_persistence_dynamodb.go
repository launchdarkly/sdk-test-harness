package sdktests

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

const (
	// Schema of the DynamoDB table
	tablePartitionKey = "namespace"
	tableName         = "sdk-contract-tests"
	tableSortKey      = "key"
	versionAttribute  = "version"
	itemJSONAttribute = "item"

	initedKey = "$inited"

	// We won't try to store items whose total size exceeds this. The DynamoDB documentation says
	// only "400KB", which probably means 400*1024, but to avoid any chance of trying to store a
	// too-large item we are rounding it down.
	dynamoDbMaxItemSize = 400000
)

type DynamoDBPersistentStore struct {
	dynamodb *dynamodb.DynamoDB
}

// {{{ PersistentStore implementation

func (d DynamoDBPersistentStore) DSN() string {
	return ""
}

func (d *DynamoDBPersistentStore) Type() servicedef.SDKConfigPersistentType {
	return servicedef.DynamoDB
}

func (d *DynamoDBPersistentStore) Reset() error {
	d.dynamodb.DeleteTable(&dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	d.dynamodb.CreateTable(&dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String(tablePartitionKey),
				AttributeType: aws.String("S"),
			},
			{
				AttributeName: aws.String(tableSortKey),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String(tablePartitionKey),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String(tableSortKey),
				KeyType:       aws.String("RANGE"),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
		TableName: aws.String(tableName),
	})

	return nil
}

func (d *DynamoDBPersistentStore) Get(prefix, key string) (string, bool, error) {
	result, err := d.dynamodb.GetItem(
		&dynamodb.GetItemInput{
			TableName: aws.String(tableName),
			Key: map[string]*dynamodb.AttributeValue{
				tablePartitionKey: {S: aws.String(addPrefix(prefix, key))},
				tableSortKey:      {S: aws.String(addPrefix(prefix, key))},
			},
		})

	if err != nil || result == nil {
		return "", false, err
	} else if result.Item == nil {
		return "", false, nil
	} else if key == initedKey {
		return "", true, nil
	}

	if len(result.Item) != 1 {
		return "", false, nil
	}

	return *result.Item[itemJSONAttribute].S, true, nil
}

func (d *DynamoDBPersistentStore) GetMap(prefix, key string) (map[string]string, error) {
	query := &dynamodb.QueryInput{
		TableName:      aws.String(tableName),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]*dynamodb.Condition{
			tablePartitionKey: {
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
		itemKey := *item[tableSortKey].S
		results[itemKey] = *item[itemJSONAttribute].S
	}

	return results, nil
}

func (d *DynamoDBPersistentStore) WriteMap(prefix, key string, data map[string]string) error {
	unusedKeys := make(map[string]bool)

	condition := dynamodb.Condition{
		ComparisonOperator: aws.String("EQ"),
		AttributeValueList: []*dynamodb.AttributeValue{{
			S: aws.String(addPrefix(prefix, key)),
		}},
	}

	// Read in all the old keys first
	query := &dynamodb.QueryInput{
		TableName:      aws.String(tableName),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]*dynamodb.Condition{
			tablePartitionKey: &condition,
		},
	}

	response, err := d.dynamodb.Query(query)
	if err != nil {
		return err
	}

	for _, item := range response.Items {
		itemKey := item[tableSortKey].String()
		unusedKeys[itemKey] = true
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
					tablePartitionKey: {S: aws.String(addPrefix(prefix, key))},
					tableSortKey:      {S: aws.String(k)},
					itemJSONAttribute: {S: aws.String(v)},
					versionAttribute:  {N: aws.String(strconv.Itoa(versioned.Version))},
				},
			},
		})
		delete(unusedKeys, k)
	}

	for k := range unusedKeys {
		if k == initedKey {
			continue
		}
		delKey := map[string]*dynamodb.AttributeValue{
			tablePartitionKey: {S: aws.String(addPrefix(prefix, key))},
			tableSortKey:      {S: aws.String(k)},
		}
		requests = append(requests, &dynamodb.WriteRequest{
			DeleteRequest: &dynamodb.DeleteRequest{Key: delKey},
		})
	}

	// Now set the special key that we check in InitializedInternal()
	requests = append(requests, &dynamodb.WriteRequest{
		PutRequest: &dynamodb.PutRequest{Item: map[string]*dynamodb.AttributeValue{
			tablePartitionKey: {S: aws.String(addPrefix(prefix, initedKey))},
			tableSortKey:      {S: aws.String(initedKey)},
		}},
	})

	if err := batchWriteRequests(context.Background(), d.dynamodb, tableName, requests); err != nil {
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

// }}}

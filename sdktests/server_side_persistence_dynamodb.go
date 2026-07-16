package sdktests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

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
	dynamodb *dynamodb.Client
	endpoint string
}

func (d *DynamoDBPersistentStore) DSN() string {
	return d.endpoint
}

func (d *DynamoDBPersistentStore) Type() servicedef.SDKConfigPersistentType {
	return servicedef.DynamoDB
}

func (d *DynamoDBPersistentStore) Reset() error {
	ctx := context.Background()

	_, err := d.dynamodb.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(dynamoDBTableName)})
	var notFound *types.ResourceNotFoundException
	if err != nil && !errors.As(err, &notFound) {
		return err
	}

	_, err = d.dynamodb.CreateTable(ctx, &dynamodb.CreateTableInput{
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String(dynamoDBTablePartitionKey),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String(dynamoDBTableSortKey),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String(dynamoDBTablePartitionKey),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String(dynamoDBTableSortKey),
				KeyType:       types.KeyTypeRange,
			},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
		TableName: aws.String(dynamoDBTableName),
	})
	return err
}

func (d *DynamoDBPersistentStore) Get(prefix, key string) (o.Maybe[string], error) {
	result, err := d.dynamodb.GetItem(
		context.Background(),
		&dynamodb.GetItemInput{
			TableName: aws.String(dynamoDBTableName),
			Key: map[string]types.AttributeValue{
				dynamoDBTablePartitionKey: &types.AttributeValueMemberS{Value: addPrefix(prefix, key)},
				dynamoDBTableSortKey:      &types.AttributeValueMemberS{Value: addPrefix(prefix, key)},
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

	return o.Some(result.Item[dynamoDBItemJSONAttribute].(*types.AttributeValueMemberS).Value), nil
}

func (d *DynamoDBPersistentStore) GetMap(prefix, key string) (map[string]string, error) {
	query := &dynamodb.QueryInput{
		TableName:      aws.String(dynamoDBTableName),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]types.Condition{
			dynamoDBTablePartitionKey: {
				ComparisonOperator: types.ComparisonOperatorEq,
				AttributeValueList: []types.AttributeValue{
					&types.AttributeValueMemberS{Value: addPrefix(prefix, key)},
				},
			},
		},
	}

	results := map[string]string{}
	response, err := d.dynamodb.Query(context.Background(), query)
	if err != nil {
		return results, err
	}

	for _, item := range response.Items {
		itemKey := item[dynamoDBTableSortKey].(*types.AttributeValueMemberS).Value
		results[itemKey] = item[dynamoDBItemJSONAttribute].(*types.AttributeValueMemberS).Value
	}

	return results, nil
}

func (d *DynamoDBPersistentStore) WriteMap(prefix, key string, data map[string]string) error {
	unusedKeys := make(map[string]struct{})

	condition := types.Condition{
		ComparisonOperator: types.ComparisonOperatorEq,
		AttributeValueList: []types.AttributeValue{
			&types.AttributeValueMemberS{Value: addPrefix(prefix, key)},
		},
	}

	// Read in all the old keys first
	query := &dynamodb.QueryInput{
		TableName:      aws.String(dynamoDBTableName),
		ConsistentRead: aws.Bool(true),
		KeyConditions: map[string]types.Condition{
			dynamoDBTablePartitionKey: condition,
		},
	}

	response, err := d.dynamodb.Query(context.Background(), query)
	if err != nil {
		return err
	}

	for _, item := range response.Items {
		itemKey := item[dynamoDBTableSortKey].(*types.AttributeValueMemberS).Value
		unusedKeys[itemKey] = struct{}{}
	}

	requests := make([]types.WriteRequest, 0)

	for k, v := range data {
		var versioned struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal([]byte(v), &versioned); err != nil {
			return err
		}
		requests = append(requests, types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: map[string]types.AttributeValue{
					dynamoDBTablePartitionKey: &types.AttributeValueMemberS{Value: addPrefix(prefix, key)},
					dynamoDBTableSortKey:      &types.AttributeValueMemberS{Value: k},
					dynamoDBItemJSONAttribute: &types.AttributeValueMemberS{Value: v},
					dynamoDBVersionAttribute:  &types.AttributeValueMemberN{Value: strconv.Itoa(versioned.Version)},
				},
			},
		})
		delete(unusedKeys, k)
	}

	for k := range unusedKeys {
		if k == persistenceInitedKey {
			continue
		}
		delKey := map[string]types.AttributeValue{
			dynamoDBTablePartitionKey: &types.AttributeValueMemberS{Value: addPrefix(prefix, key)},
			dynamoDBTableSortKey:      &types.AttributeValueMemberS{Value: k},
		}
		requests = append(requests, types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{Key: delKey},
		})
	}

	// Now set the special key that we check in InitializedInternal()
	requests = append(requests, types.WriteRequest{
		PutRequest: &types.PutRequest{Item: map[string]types.AttributeValue{
			dynamoDBTablePartitionKey: &types.AttributeValueMemberS{Value: addPrefix(prefix, persistenceInitedKey)},
			dynamoDBTableSortKey:      &types.AttributeValueMemberS{Value: persistenceInitedKey},
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
	client *dynamodb.Client,
	table string,
	requests []types.WriteRequest,
) error {
	for len(requests) > 0 {
		batchSize := int(math.Min(float64(len(requests)), 25))
		batch := requests[:batchSize]
		requests = requests[batchSize:]

		_, err := client.BatchWriteItem(context.Background(), &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{table: batch},
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

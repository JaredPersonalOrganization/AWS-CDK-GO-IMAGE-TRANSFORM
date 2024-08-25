package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var authTableName = os.Getenv("AUTH_TABLE_NAME")

var dynamo *dynamodb.Client

func InitConfig() (aws.Config, error) {
	return config.LoadDefaultConfig(context.TODO())
}

func InitDynamo(config aws.Config) *dynamodb.Client {
	return dynamodb.NewFromConfig(config)
}

type S3Object struct {
	Key string `json:"key"`
}

type S3Entity struct {
	Object S3Object `json:"object"`
}

type Record struct {
	S3 S3Entity `json:"s3"`
}

type S3Event struct {
	Records []Record `json:"Records"`
}

type StatusValue struct {
	Status string `json:":status"`
}

type Item struct {
	ObjectName string `json:"object-name"`
}

func createKey(Pk, Sk string) (map[string]types.AttributeValue, error) {
	pk, err := attributevalue.Marshal(Pk)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal pk: %v", err)
	}
	sk, err := attributevalue.Marshal(Sk)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sk: %v", err)
	}
	return map[string]types.AttributeValue{"pk": pk, "sk": sk}, nil
}

func lambdaHandler(ctx context.Context, sqsEvent events.SQSEvent) error {

	awsConfig, err := InitConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}
	dynamo = InitDynamo(awsConfig)
	var batchDLQErrors []error

	for _, message := range sqsEvent.Records {

		var s3Event S3Event
		err := json.Unmarshal([]byte(message.Body), &s3Event)

		if err != nil {
			batchDLQErrors = append(batchDLQErrors, fmt.Errorf("error unmarshalling message: %v", err))
			continue
		}

		for _, record := range s3Event.Records {

			update := expression.Set(expression.Name("Status"), expression.Value("broken"))
			expr, err := expression.NewBuilder().WithUpdate(update).Build()
			if err != nil {

				batchDLQErrors = append(batchDLQErrors, fmt.Errorf("failed to build expression: %v", err))
				continue
			}

			key, err := createKey(record.S3.Object.Key, "metadata")
			if err != nil {
				batchDLQErrors = append(batchDLQErrors, fmt.Errorf("failed to create key: %v", err))
				continue
			}

			input := &dynamodb.UpdateItemInput{
				TableName:                 aws.String(authTableName),
				Key:                       key,
				ExpressionAttributeNames:  expr.Names(),
				ExpressionAttributeValues: expr.Values(),
				UpdateExpression:          expr.Update(),
				ReturnValues:              types.ReturnValueUpdatedNew,
				ConditionExpression:       aws.String("attribute_exists(pk) AND attribute_exists(sk)"),
			}

			_, err = dynamo.UpdateItem(context.TODO(), input)
			if err != nil {

				batchDLQErrors = append(batchDLQErrors, fmt.Errorf("failed to updated dynamodb item:  %v", err))
				continue
			}
		}
	}

	return errors.Join(batchDLQErrors...)
}

func main() {
	lambda.Start(lambdaHandler)
}

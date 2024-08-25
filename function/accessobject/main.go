package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var authTableName = os.Getenv("AUTH_TABLE_NAME")
var outputBucketName = os.Getenv("OUTPUT_BUCKET_NAME")

var svc *s3.Client
var dynamo *dynamodb.Client

func InitConfig() (aws.Config, error) {
	return config.LoadDefaultConfig(context.TODO())
}

func InitDynamo(config aws.Config) *dynamodb.Client {
	return dynamodb.NewFromConfig(config)
}

func InitS3(config aws.Config) *s3.Client {
	return s3.NewFromConfig(config)
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

func CreatePresignedURL(uniqueID string) (string, error) {

	presignClient := s3.NewPresignClient(svc)
	presignedURL, err := presignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(outputBucketName),
		Key:    aws.String(uniqueID),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = time.Duration(60 * int64(time.Second))
	})

	if err != nil {
		return "", fmt.Errorf("failed to create presigned url: %v", err)
	}
	return presignedURL.URL, nil
}

func CheckTableStatus(uniqueID string) (events.APIGatewayProxyResponse, error) {

	key, err := createKey(uniqueID, "metadata")
	if err != nil {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusInternalServerError,
			},
			fmt.Errorf("failed to create key: %s", err)
	}

	response, err := dynamo.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName:      aws.String(authTableName),
		Key:            key,
		ConsistentRead: aws.Bool(true),
	})

	if err != nil {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusInternalServerError,
			},
			fmt.Errorf("error while trying to get item: %s", err)
	}

	if response != nil {
		if status, ok := response.Item["Status"]; ok {
			if statusString, ok := status.(*types.AttributeValueMemberS); ok {

				switch statusString.Value {
				case "processing":
					return events.APIGatewayProxyResponse{
							StatusCode: http.StatusTooEarly,
						},
						nil
				case "broken":
					return events.APIGatewayProxyResponse{
							StatusCode: http.StatusBadRequest,
						},
						fmt.Errorf("object is broken. need a better way to convey that")
				case "processed":
					presignedURL, err := CreatePresignedURL(uniqueID)
					if err != nil {
						return events.APIGatewayProxyResponse{
								StatusCode: http.StatusInternalServerError,
							},
							fmt.Errorf("failed to create presigned url: %s", err)
					}
					return events.APIGatewayProxyResponse{
							StatusCode: http.StatusOK,
							Body:       presignedURL,
						},
						nil
				default:
					panic("should be unreachable")
				}
			}
		}
		panic("table is missing status")
	}
	panic("should be impossible to reach here. The authorizer already allowed this request so such object does did in fact exist in the table. but somehow it is now gone.. strange.")
}

func lambdaHandler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	if request.HTTPMethod != "GET" {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusMethodNotAllowed,
			},
			fmt.Errorf("invalid http method")
	}

	objectName, ok := request.QueryStringParameters["object-name"]
	if !ok {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusBadRequest,
			},
			fmt.Errorf("missing object-name query parameter")
	}

	fmt.Printf("user is requesting access to: %s \n", objectName)

	awsConfig, err := InitConfig()
	if err != nil {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusInternalServerError,
			},
			fmt.Errorf("failed to initialize AWS config: %v", err)
	}

	dynamo = InitDynamo(awsConfig)
	svc = InitS3(awsConfig)
	return CheckTableStatus(objectName)
}

func main() {

	lambda.Start(lambdaHandler)
}

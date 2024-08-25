package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/google/uuid"
)

var bucketName = os.Getenv("INPUT_BUCKET_NAME")
var authName = os.Getenv("AUTH_TABLE_NAME")

var svc *s3.Client
var dynamo *dynamodb.Client

type Transform struct {
	Name   string   `dynamodbav:"Name" json:"Name"`
	Params []string `dynamodbav:"Params" json:"Params"`
}

type OutputItem struct {
	Pk          string      `dynamodbav:"pk" json:"pk"`
	Sk          string      `dynamodbav:"sk" json:"sk"`
	SourceIP    string      `dynamodbav:"SourceIP" json:"SourceIP"`
	Status      string      `dynamodbav:"Status" json:"Status"`
	ContentType string      `dynamodbav:"ContentType" json:"ContentType"`
	Transforms  []Transform `dynamodbav:"Transforms" json:"Transforms"`
}

type InputItem struct {
	ObjectName string      `dynamodbav:"ObjectName"`
	Transforms []Transform `dynamodbav:"Transforms"`
}

func getResourceSuffix(resource string) *string {
	allowedSuffixes := []string{".jpg", ".jpeg", ".png", ".gif"}
	for _, suffix := range allowedSuffixes {
		if strings.HasSuffix(resource, suffix) {
			return &suffix
		}
	}
	return nil
}

func InitConfig() (aws.Config, error) {
	return config.LoadDefaultConfig(context.TODO())
}

func InitDynamo(config aws.Config) *dynamodb.Client {
	return dynamodb.NewFromConfig(config)
}

func InitS3(config aws.Config) *s3.Client {
	return s3.NewFromConfig(config)
}

func lambdaHandler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	if request.HTTPMethod != "POST" {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusMethodNotAllowed,
			},
			fmt.Errorf("invalid http method")
	}

	var inputItem InputItem
	err := json.Unmarshal([]byte(request.Body), &inputItem)
	if err != nil {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusInternalServerError,
			},
			fmt.Errorf("failed to parse request body: %v", err)
	}

	resourceSuffix := getResourceSuffix(inputItem.ObjectName)
	if resourceSuffix == nil {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusUnsupportedMediaType,
			},
			fmt.Errorf("unsupported resource type")
	}

	awsConfig, err := InitConfig()
	if err != nil {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusInternalServerError,
			},
			fmt.Errorf("failed to initialize aws config: %v", err)
	}

	svc = InitS3(awsConfig)
	dynamo = InitDynamo(awsConfig)

	uniqueObjectName := "image-" + uuid.New().String() + *resourceSuffix

	presignClient := s3.NewPresignClient(svc)

	presignedURL, err := presignClient.PresignPutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(uniqueObjectName),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = time.Duration(60 * int64(time.Second))
	})

	if err != nil {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusInternalServerError,
			},
			fmt.Errorf("failed to generate presigned url: %v", err) // todo
	}

	outputItem := OutputItem{
		Pk:          uniqueObjectName,
		Sk:          "metadata",
		SourceIP:    request.RequestContext.Identity.SourceIP,
		Status:      "processing",
		ContentType: *resourceSuffix,
		Transforms:  inputItem.Transforms,
	}

	av, err := attributevalue.MarshalMap(outputItem)
	if err != nil {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusInternalServerError,
			},
			fmt.Errorf("failed to marshal item: %v", err)
	}

	_, err = dynamo.PutItem(context.TODO(), &dynamodb.PutItemInput{
		TableName: aws.String(authName),
		Item:      av,
	})

	if err != nil {
		return events.APIGatewayProxyResponse{
				StatusCode: http.StatusInternalServerError,
			},
			fmt.Errorf("failed to put item in dynamodb: %v", err)
	}

	return events.APIGatewayProxyResponse{
		Headers:    map[string]string{"object-name": uniqueObjectName},
		Body:       presignedURL.URL,
		StatusCode: http.StatusOK,
	}, nil
}

func main() {
	lambda.Start(lambdaHandler)
}

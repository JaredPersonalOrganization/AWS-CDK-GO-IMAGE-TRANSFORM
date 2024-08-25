package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
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

func isAuthorized(currentIP, uniqueID string) bool {

	key, err := createKey(uniqueID, "metadata")
	if err != nil {
		fmt.Printf("failed to create key: %s", err)
		return false
	}

	response, err := dynamo.GetItem(context.TODO(), &dynamodb.GetItemInput{
		TableName:      aws.String(authTableName),
		Key:            key,
		ConsistentRead: aws.Bool(true),
	})

	if err != nil {
		fmt.Printf("error calling GetItem: %s", err)
		return false
	}

	if response != nil {
		if originalIP, ok := response.Item["SourceIP"]; ok {

			originalIPString, ok := originalIP.(*types.AttributeValueMemberS)

			if ok {
				if originalIPString.Value == currentIP {
					fmt.Printf("The stored source IP %s matches current source IP %s", originalIPString.Value, currentIP)
					return true
				} else {
					fmt.Printf("The stored source IP %s does not match current source IP %s", originalIPString.Value, currentIP)
					return false
				}
			} else {
				panic("failed to cast originalIP to *types.AttributeValueMemberS")
			}

		} else {
			panic("failed to find source ip inside of item map")
		}
	}

	fmt.Printf("user attemped to access a no existent item with unique_id:%s", uniqueID)
	return false
}

func GeneratePolicy(principalId, effect, resource string) events.APIGatewayCustomAuthorizerResponse {
	authResponse := events.APIGatewayCustomAuthorizerResponse{PrincipalID: principalId}

	if effect != "" && resource != "" {
		authResponse.PolicyDocument = events.APIGatewayCustomAuthorizerPolicy{
			Version: "2012-10-17",
			Statement: []events.IAMPolicyStatement{
				{
					Action:   []string{"execute-api:Invoke"},
					Effect:   effect,
					Resource: []string{resource},
				},
			},
		}
	}
	return authResponse
}

func lambdaHandler(ctx context.Context, event events.APIGatewayCustomAuthorizerRequestTypeRequest) (events.APIGatewayCustomAuthorizerResponse, error) {

	awsConfig, err := InitConfig()
	if err != nil {
		panic("failed to load config")
	}

	dynamo = InitDynamo(awsConfig)

	if objectName, ok := event.QueryStringParameters["object-name"]; ok {

		if isAuthorized(event.RequestContext.Identity.SourceIP, objectName) {
			return GeneratePolicy("user", "Allow", event.MethodArn), nil
		} else {
			return GeneratePolicy("user", "Deny", event.MethodArn), nil
		}
	}

	return GeneratePolicy("user", "Deny", event.MethodArn), nil
}

func main() {
	lambda.Start(lambdaHandler)
}

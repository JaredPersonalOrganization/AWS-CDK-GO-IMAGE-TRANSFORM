package main

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsapigateway"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	iam "github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	lambda "github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	lambdaevent "github.com/aws/aws-cdk-go/awscdk/v2/awslambdaeventsources"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	s3n "github.com/aws/aws-cdk-go/awscdk/v2/awss3notifications"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	awssnssub "github.com/aws/aws-cdk-go/awscdk/v2/awssnssubscriptions"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssqs"
	awslambdago "github.com/aws/aws-cdk-go/awscdklambdagoalpha/v2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type CdkImageTransformStackProps struct {
	awscdk.StackProps
}

func NewCdkImageTransformStack(scope constructs.Construct, id string, props *CdkImageTransformStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}
	stack := awscdk.NewStack(scope, &id, &sprops)

	inputBucket := awss3.NewBucket(stack, jsii.String("Input"), &awss3.BucketProps{
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		RemovalPolicy:     awscdk.RemovalPolicy_DESTROY,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		AutoDeleteObjects: jsii.Bool(true),
		LifecycleRules: &[]*awss3.LifecycleRule{
			{
				Enabled:    jsii.Bool(true),
				Expiration: awscdk.Duration_Days(jsii.Number(1)),
			},
		},
	})

	outputBucket := awss3.NewBucket(stack, jsii.String("Output"), &awss3.BucketProps{
		Encryption:        awss3.BucketEncryption_S3_MANAGED,
		RemovalPolicy:     awscdk.RemovalPolicy_DESTROY,
		BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL(),
		AutoDeleteObjects: jsii.Bool(true),
		LifecycleRules: &[]*awss3.LifecycleRule{
			{
				Enabled:    jsii.Bool(true),
				Expiration: awscdk.Duration_Days(jsii.Number(1)),
			},
		},
	})

	authTable := awsdynamodb.NewTable(stack, jsii.String("AuthTable"), &awsdynamodb.TableProps{
		PartitionKey:  &awsdynamodb.Attribute{Name: jsii.String("pk"), Type: awsdynamodb.AttributeType_STRING},
		SortKey:       &awsdynamodb.Attribute{Name: jsii.String("sk"), Type: awsdynamodb.AttributeType_STRING},
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	bundlingOptions := &awslambdago.BundlingOptions{
		GoBuildFlags: &[]*string{jsii.String(`-ldflags "-s -w"`)}, // -s: Strip symbols, -w: Strip debug info
	}

	dlq := awssqs.NewQueue(stack, jsii.String("BucketUploadQueueDLQ"), nil)

	// create the SQS queue
	uploadQueue := awssqs.NewQueue(stack, jsii.String("BucketUploadQueue"), &awssqs.QueueProps{
		VisibilityTimeout: awscdk.Duration_Seconds(jsii.Number(300)),
		DeadLetterQueue: &awssqs.DeadLetterQueue{
			MaxReceiveCount: jsii.Number(1),
			Queue:           dlq,
		},
	})

	// generate url lambda
	generateUrlLambda := awslambdago.NewGoFunction(stack, jsii.String("GenerateUrlLambda"), &awslambdago.GoFunctionProps{
		Architecture: lambda.Architecture_X86_64(),
		Runtime:      lambda.Runtime_PROVIDED_AL2(),
		Bundling:     bundlingOptions,
		MemorySize:   jsii.Number(128),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(10)),
		Entry:        jsii.String("function/getpresigned"),
		Environment: &map[string]*string{
			"AUTH_TABLE_NAME":   authTable.TableName(),
			"INPUT_BUCKET_NAME": inputBucket.BucketName(),
		},
	})

	generateUrlLambda.AddToRolePolicy(iam.NewPolicyStatement(&iam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("s3:PutObject"),
		},
		Resources: &[]*string{
			inputBucket.ArnForObjects(jsii.String("*")),
		},
	}))

	authTable.GrantWriteData(generateUrlLambda)

	// create image transform lambda
	transformImageLambda := awslambdago.NewGoFunction(stack, jsii.String("TransformImageLambda"), &awslambdago.GoFunctionProps{
		Architecture: lambda.Architecture_X86_64(),
		Runtime:      lambda.Runtime_PROVIDED_AL2(),
		Bundling:     bundlingOptions,
		MemorySize:   jsii.Number(256),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(300)),
		Entry:        jsii.String("function/transformimage"),
		Environment: &map[string]*string{
			"INPUT_BUCKET_NAME":  inputBucket.BucketName(),
			"OUTPUT_BUCKET_NAME": outputBucket.BucketName(),
			"AUTH_TABLE_NAME":    authTable.TableName(),
		},
	})

	transformImageLambda.AddToRolePolicy(iam.NewPolicyStatement(&iam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("s3:GetObject"),
			jsii.String("s3:PutObject"),
		},
		Resources: &[]*string{
			inputBucket.ArnForObjects(jsii.String("*")),
			outputBucket.ArnForObjects(jsii.String("*")),
		},
	}))

	authTable.GrantReadWriteData(transformImageLambda)

	accessObjectLambda := awslambdago.NewGoFunction(stack, jsii.String("AccessObjectLambda"), &awslambdago.GoFunctionProps{
		Architecture: lambda.Architecture_X86_64(),
		Runtime:      lambda.Runtime_PROVIDED_AL2(),
		Bundling:     bundlingOptions,
		MemorySize:   jsii.Number(128),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(10)),
		Entry:        jsii.String("function/accessobject"),
		Environment: &map[string]*string{
			"OUTPUT_BUCKET_NAME": outputBucket.BucketName(),
			"AUTH_TABLE_NAME":    authTable.TableName(),
		},
	})

	accessObjectLambda.AddToRolePolicy(iam.NewPolicyStatement(&iam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("s3:GetObject"),
		},
		Resources: &[]*string{
			outputBucket.ArnForObjects(jsii.String("*")),
		},
	}))

	authTable.GrantReadData(accessObjectLambda)

	dlqLambda := awslambdago.NewGoFunction(stack, jsii.String("DLQLambda"), &awslambdago.GoFunctionProps{
		Architecture: lambda.Architecture_X86_64(),
		Runtime:      lambda.Runtime_PROVIDED_AL2(),
		Bundling:     bundlingOptions,
		MemorySize:   jsii.Number(128),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(10)),
		Entry:        jsii.String("function/dlq"),
		Environment: &map[string]*string{
			"INPUT_BUCKET_NAME": inputBucket.BucketName(),
			"AUTH_TABLE_NAME":   authTable.TableName(),
		},
	})

	dlqLambda.AddToRolePolicy(iam.NewPolicyStatement(&iam.PolicyStatementProps{
		Actions: &[]*string{
			jsii.String("s3:GetObject"),
			jsii.String("sqs:ReceiveMessage"),
			jsii.String("sqs:DeleteMessage"),
		},
		Resources: &[]*string{
			inputBucket.ArnForObjects(jsii.String("*")),
			dlq.QueueArn(),
		},
	}))

	dlqLambda.AddEventSource(awslambdaeventsources.NewSqsEventSource(dlq, &awslambdaeventsources.SqsEventSourceProps{
		BatchSize: jsii.Number(10),
	}))

	authTable.GrantReadWriteData(dlqLambda)

	authorizeAccessLambda := awslambdago.NewGoFunction(stack, jsii.String("AuthorizeAccessLambda"), &awslambdago.GoFunctionProps{
		Architecture: lambda.Architecture_X86_64(),
		Runtime:      lambda.Runtime_PROVIDED_AL2(),
		Bundling:     bundlingOptions,
		MemorySize:   jsii.Number(128),
		Timeout:      awscdk.Duration_Seconds(jsii.Number(10)),
		Entry:        jsii.String("function/authorizeaccess"),
		Environment: &map[string]*string{
			"AUTH_TABLE_NAME": authTable.TableName(),
		},
	})

	authTable.GrantReadData(authorizeAccessLambda)

	sqsSubscription := awssnssub.NewSqsSubscription(uploadQueue, &awssnssub.SqsSubscriptionProps{
		RawMessageDelivery: jsii.Bool(true),
	})

	// create the SNS topic
	uploadEventTopic := awssns.NewTopic(stack, jsii.String("UploadEventTopic"), nil)
	uploadEventTopic.AddSubscription(sqsSubscription)

	// add the event notification
	inputBucket.AddEventNotification(awss3.EventType_OBJECT_CREATED_PUT, s3n.NewSnsDestination(uploadEventTopic), &awss3.NotificationKeyFilter{
		Prefix: jsii.String("image-"),
	})

	invokeEventSource := lambdaevent.NewSqsEventSource(uploadQueue, &lambdaevent.SqsEventSourceProps{
		BatchSize:      jsii.Number(10),
		Enabled:        jsii.Bool(true),
		MaxConcurrency: jsii.Number(10),
	})

	transformImageLambda.AddEventSource(invokeEventSource)

	auth := awsapigateway.NewRequestAuthorizer(stack, jsii.String("authapi"), &awsapigateway.RequestAuthorizerProps{
		Handler: authorizeAccessLambda,
		IdentitySources: &[]*string{
			awsapigateway.IdentitySource_QueryString(jsii.String("object-name")),
		},
	})

	api := awsapigateway.NewRestApi(stack, jsii.String("ApiGateway"), &awsapigateway.RestApiProps{
		RestApiName: jsii.String("ImageTransformRestAPI"), // change name
		DeployOptions: &awsapigateway.StageOptions{
			LoggingLevel: awsapigateway.MethodLoggingLevel_INFO,
		},
		EndpointConfiguration: &awsapigateway.EndpointConfiguration{
			Types: &[]awsapigateway.EndpointType{
				awsapigateway.EndpointType_EDGE,
			},
		},
	})

	response := awsapigateway.MethodResponse{
		StatusCode:     jsii.String("200"),
		ResponseModels: &map[string]awsapigateway.IModel{"application/json": awsapigateway.Model_EMPTY_MODEL()},
	}

	generateUrlIntegration := awsapigateway.NewLambdaIntegration(generateUrlLambda, nil)

	generateUrlResource := api.Root().AddResource(jsii.String("generate-url"), nil)
	postmethod := generateUrlResource.AddMethod(jsii.String("POST"), generateUrlIntegration, &awsapigateway.MethodOptions{
		AuthorizationType: awsapigateway.AuthorizationType_NONE,
	})
	postmethod.AddMethodResponse(&response)

	accessObjectIntegration := awsapigateway.NewLambdaIntegration(accessObjectLambda, nil)

	accessObjectResource := api.Root().AddResource(jsii.String("access-object"), nil)
	getmethod := accessObjectResource.AddMethod(jsii.String("GET"), accessObjectIntegration, &awsapigateway.MethodOptions{
		AuthorizationType: awsapigateway.AuthorizationType_CUSTOM,
		Authorizer:        auth,
	})
	getmethod.AddMethodResponse(&response)
	
	return stack
}

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	NewCdkImageTransformStack(app, "CdkImageTransformStack", &CdkImageTransformStackProps{
		awscdk.StackProps{
			Env: env(),
		},
	})

	app.Synth(nil)
}

// env determines the AWS environment (account+region) in which our stack is to
// be deployed. For more information see: https://docs.aws.amazon.com/cdk/latest/guide/environments.html
func env() *awscdk.Environment {
	// If unspecified, this stack will be "environment-agnostic".
	// Account/Region-dependent features and context lookups will not work, but a
	// single synthesized template can be deployed anywhere.
	//---------------------------------------------------------------------------
	return nil

	// Uncomment if you know exactly what account and region you want to deploy
	// the stack to. This is the recommendation for production stacks.
	//---------------------------------------------------------------------------
	// return &awscdk.Environment{
	//  Account: jsii.String("123456789012"),
	//  Region:  jsii.String("us-east-1"),
	// }

	// Uncomment to specialize this stack for the AWS Account and Region that are
	// implied by the current CLI configuration. This is recommended for dev
	// stacks.
	//---------------------------------------------------------------------------
	// return &awscdk.Environment{
	//  Account: jsii.String(os.Getenv("CDK_DEFAULT_ACCOUNT")),
	//  Region:  jsii.String(os.Getenv("CDK_DEFAULT_REGION")),
	// }
}

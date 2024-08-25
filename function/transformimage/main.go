package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strconv"

	"github.com/anthonynsimon/bild/effect"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var inputBucketName = os.Getenv("INPUT_BUCKET_NAME")
var outputBucketName = os.Getenv("OUTPUT_BUCKET_NAME")
var tableName = os.Getenv("AUTH_TABLE_NAME")

const MaxImageWidth int = 7680
const MaxImageHeight int = 4320
const MaxImageSizeBytes int = MaxImageWidth * MaxImageHeight * 4

var svc *s3.Client
var dynamo *dynamodb.Client

type S3BucketJson struct {
	Name string `json:"name"`
}
type S3ObjectJson struct {
	Key       string `json:"key"`
	Size      int    `json:"size"`
	ETag      string `json:"eTag"`
	Sequencer string `json:"sequencer"`
}

type S3Json struct {
	Bucket S3BucketJson `json:"bucket"`
	Object S3ObjectJson `json:"object"`
}

type RecordJson struct {
	EventTime string `json:"eventTime"`
	S3        S3Json `json:"s3"`
}

type Event struct {
	Records []RecordJson `json:"Records"`
}

type Transform struct {
	Name   string   `dynamodbav:"Name" json:"Name"`
	Params []string `dynamodbav:"Params,omitempty" json:"Params,omitempty"`
}

type InputItem struct {
	Pk          string      `dynamodbav:"pk" json:"pk"`
	Sk          string      `dynamodbav:"sk" json:"sk"`
	SourceIP    string      `dynamodbav:"SourceIP" json:"SourceIP"`
	Status      string      `dynamodbav:"Status" json:"Status"`
	ContentType string      `dynamodbav:"ContentType" json:"ContentType"`
	Transforms  []Transform `dynamodbav:"Transforms" json:"Transforms"`
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

type ItemKey struct {
	Pk string `json:"pk"`
	Sk string `json:"sk"`
}

type ItemStatus struct {
	Status string `json:"status"`
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

func imageToRGBA(src image.Image) *image.RGBA {

	if dst, ok := src.(*image.RGBA); ok {
		return dst
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func DilateImage(img image.Image, params []string) (image.Image, error) {
	if len(params) != 1 {
		return nil, errors.New("invalid number of parameters")
	}
	intensity, err := strconv.ParseFloat(params[0], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dilate value: %v", err)
	}
	return effect.Dilate(img, intensity), nil
}

func EdgeDetection(img image.Image, params []string) (image.Image, error) {

	if len(params) != 1 {
		return nil, errors.New("invalid number of parameters")
	}
	radius, err := strconv.ParseFloat(params[0], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dilate value: %v", err)
	}
	if radius > 1.0 {
		radius = 1.0
	} else if radius < 0.0 {
		radius = 0.0
	}

	return effect.EdgeDetection(img, radius), nil
}

func Erode(img image.Image, params []string) (image.Image, error) {

	if len(params) != 1 {
		return nil, errors.New("invalid number of parameters")
	}
	erosion, err := strconv.ParseFloat(params[0], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dilate value: %v", err)
	}
	return effect.Erode(img, erosion), nil
}

func Median(img image.Image, params []string) (image.Image, error) {

	if len(params) != 1 {
		return nil, errors.New("invalid number of parameters")
	}
	median, err := strconv.ParseFloat(params[0], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse median value: %v", err)
	}
	return effect.Median(img, median), nil
}

func Emboss(img image.Image) image.Image {

	return effect.Emboss(img)
}

func Grayscale(img image.Image) image.Image {

	return effect.Grayscale(img)
}

func Invert(img image.Image) image.Image {

	return effect.Invert(img)
}

func Sepia(img image.Image) image.Image {

	return effect.Sepia(img)
}

func Sharpen(img image.Image) image.Image {

	return effect.Sharpen(img)
}

func Sobel(img image.Image) image.Image {

	return effect.Sobel(img)
}

func TransformImage(img image.Image, item *InputItem) (image.Image, error) {

	var err error
	img = imageToRGBA(img)

	for _, transform := range item.Transforms {
		switch transform.Name {
		case "dilate":
			if img, err = DilateImage(img, transform.Params); err != nil {
				return nil, fmt.Errorf("failed dilate: %v", err)
			}
		case "edgedetection":
			if img, err = EdgeDetection(img, transform.Params); err != nil {
				return nil, fmt.Errorf("failed edgedetection: %v", err)
			}
		case "erode":
			if img, err = Erode(img, transform.Params); err != nil {
				return nil, fmt.Errorf("failed erode: %v", err)
			}
		case "median":
			if img, err = Median(img, transform.Params); err != nil {
				return nil, fmt.Errorf("failed median: %v", err)
			}
		case "emboss":
			img = Emboss(img)
		case "grayscale":
			img = Grayscale(img)
		case "invert":
			img = Invert(img)
		case "sepia":
			img = Sepia(img)
		case "sharpen":
			img = Sharpen(img)
		case "sobel":
			img = Sobel(img)
		default:
			if transform.Name != "quality" {
				fmt.Printf("unknown transform: %s\n", transform.Name)
			}
		}
	}
	return img, nil
}

func EncodeImage(img *image.RGBA, destBuffer *bytes.Buffer, inputItem *InputItem) error {

	switch inputItem.ContentType {
	case ".jpeg", ".jpg":
		return jpeg.Encode(destBuffer, img, nil) // nil for default quality. I may add the parameter
	case ".png":
		return png.Encode(destBuffer, img)
	case ".gif":
		return gif.Encode(destBuffer, img, nil)
	default:
		fmt.Printf("unknown content type: %s\n", inputItem.ContentType)
		panic("shouldn't have reached here")
	}
}

func lambdaHandler(ctx context.Context, sqsEvent events.SQSEvent) (events.SQSEventResponse, error) {

	awsConfig, err := InitConfig()
	if err != nil {
		return events.SQSEventResponse{}, fmt.Errorf("failed to load aws config: %v", err)
	}

	dynamo = InitDynamo(awsConfig)
	svc = InitS3(awsConfig)

	var batchItemFailures []events.SQSBatchItemFailure
	var batchItemErrors []error

	for _, message := range sqsEvent.Records {
		fmt.Printf("The message %s for event source %s = %s \n", message.MessageId, message.EventSource, message.Body)

		var event Event
		err := json.Unmarshal([]byte(message.Body), &event)
		if err != nil {
			return events.SQSEventResponse{}, fmt.Errorf("failed to parse request body: %v", err)
		}

		if len(event.Records) != 1 { // the json of body is an array called records

			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("event.Record was not equal to one %d", len(event.Records)))
			continue
		}

		record := event.Records[0]

		if record.S3.Bucket.Name != inputBucketName {

			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("invalid input bucket name"))
			continue
		}

		if record.S3.Object.Size > MaxImageSizeBytes {

			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("image size exceeds maximum allowed size"))
			continue
		}

		object, err := svc.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: aws.String(inputBucketName),
			Key:    aws.String(record.S3.Object.Key),
		})

		if err != nil {

			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to get object: %v", err))
			continue
		}

		defer object.Body.Close()

		buffer, err := io.ReadAll(object.Body)
		if err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to copy image buffer: %v", err))
			continue
		}

		imageReader := bytes.NewReader(buffer)
		config, _, err := image.DecodeConfig(imageReader)

		if err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to decode image config: %v", err))
			continue
		}

		if config.Width > MaxImageHeight || config.Height > MaxImageHeight {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("image dimensions exceed maximum allowed dimensions"))
			continue
		}

		_, err = imageReader.Seek(0, 0)
		if err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to seek to beginning of stream: %v", err))
			continue
		}

		srcImage, _, err := image.Decode(imageReader)
		if err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to decode image: %v", err))
			continue
		}

		key, err := createKey(record.S3.Object.Key, "metadata")
		if err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to create key: %v", err))
			continue
		}

		response, err := dynamo.GetItem(context.TODO(), &dynamodb.GetItemInput{
			TableName: aws.String(tableName),
			Key:       key,
		})

		if err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to get dynamodb item: %v", err))
			continue
		}

		var item InputItem
		err = attributevalue.UnmarshalMap(response.Item, &item)
		if err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to unmarshal dynamodb item: %v", err))
		}
		if item.Pk == "" {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("item not found: %v", err))
		}

		destImage, err := TransformImage(srcImage, &item)
		if err != nil {
			batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: message.MessageId,
			})
			batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to transform image:  %v", err))
			continue
		}

		if dst, ok := destImage.(*image.RGBA); ok {

			var imageBuf bytes.Buffer
			err = EncodeImage(dst, &imageBuf, &item)
			if err != nil {
				batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: message.MessageId,
				})
				batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to encode image: %v", err))
				continue
			}

			_, err = svc.PutObject(context.TODO(), &s3.PutObjectInput{
				Bucket: aws.String(outputBucketName),
				Key:    aws.String(record.S3.Object.Key),
				Body:   bytes.NewReader(imageBuf.Bytes()),
			})

			if err != nil {
				batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: message.MessageId,
				})
				batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to put object: %v", err))
				continue
			}

			update := expression.Set(expression.Name("Status"), expression.Value("processed"))
			expr, err := expression.NewBuilder().WithUpdate(update).Build()
			if err != nil {
				batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: message.MessageId,
				})
				batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to build expression:  %v", err))
				continue
			}

			input := &dynamodb.UpdateItemInput{
				TableName:                 aws.String(tableName),
				Key:                       key,
				ExpressionAttributeNames:  expr.Names(),
				ExpressionAttributeValues: expr.Values(),
				UpdateExpression:          expr.Update(),
				ReturnValues:              types.ReturnValueUpdatedNew,
			}

			_, err = dynamo.UpdateItem(context.TODO(), input)
			if err != nil {
				batchItemFailures = append(batchItemFailures, events.SQSBatchItemFailure{
					ItemIdentifier: message.MessageId,
				})
				batchItemErrors = append(batchItemErrors, fmt.Errorf("failed to updated dynamodb item:  %v", err))
				continue
			}

		} else {
			panic("shouldn't have reached here")
		}
	}

	return events.SQSEventResponse{
		BatchItemFailures: batchItemFailures,
	}, errors.Join(batchItemErrors...)
}

func main() {
	lambda.Start(lambdaHandler)
}

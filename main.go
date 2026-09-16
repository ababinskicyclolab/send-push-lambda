package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"google.golang.org/api/option"
)

var (
	messagingClient *messaging.Client

	credentialsPath = os.Getenv("FCM_CREDENTIALS_PATH")
)

// pushMessage is the SQS message body contract: a generic envelope with no
// domain knowledge (dose reminder, share invite, ...) — the producer decides
// the Android channel and builds Data, this Lambda just relays it to FCM.
type pushMessage struct {
	Token          string            `json:"token"`
	Title          string            `json:"title"`
	Body           string            `json:"body"`
	AndroidChannel string            `json:"android_channel"`
	Data           map[string]string `json:"data"`
}

func init() {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic(err)
	}

	ssmClient := ssm.NewFromConfig(cfg)
	out, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(credentialsPath),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		panic(fmt.Sprintf("failed to fetch FCM credentials from SSM: %v", err))
	}

	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsJSON([]byte(aws.ToString(out.Parameter.Value))))
	if err != nil {
		panic(err)
	}
	messagingClient, err = app.Messaging(ctx)
	if err != nil {
		panic(err)
	}
}

func send(ctx context.Context, msg pushMessage) error {
	_, err := messagingClient.Send(ctx, &messaging.Message{
		Token: msg.Token,
		Notification: &messaging.Notification{
			Title: msg.Title,
			Body:  msg.Body,
		},
		Data: msg.Data,
		Android: &messaging.AndroidConfig{
			Notification: &messaging.AndroidNotification{
				ChannelID: msg.AndroidChannel,
			},
		},
	})
	return err
}

// handler sends every message in the batch to FCM. A failure — bad JSON or
// an FCM error — is reported back via BatchItemFailures instead of a
// top-level error, so only that message (not the whole batch) is
// redelivered and eventually moved to the DLQ. See
// https://docs.aws.amazon.com/lambda/latest/dg/with-sqs.html#services-sqs-batchfailurereporting
func handler(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	var failures []events.SQSBatchItemFailure

	for _, record := range event.Records {
		var msg pushMessage
		if err := json.Unmarshal([]byte(record.Body), &msg); err != nil {
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
			continue
		}
		if err := send(ctx, msg); err != nil {
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
		}
	}

	return events.SQSEventResponse{BatchItemFailures: failures}, nil
}

func main() {
	lambda.Start(handler)
}

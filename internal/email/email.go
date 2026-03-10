// Package email provides a thin wrapper around AWS SES v2 for transactional email.
//
// Required env vars:
//   - EMAIL_FROM      — verified sender address (e.g. "Winnow <noreply@winnow-api.xferops.dev>")
//   - AWS_REGION      — used by the default AWS config loader
//
// If EMAIL_FROM is unset, Send() returns nil without sending (safe for local dev).
package email

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// Client wraps SES v2.
type Client struct {
	ses  *sesv2.Client
	from string
}

// New creates a Client from the ambient AWS config.
// Returns (nil, nil) when EMAIL_FROM is not set — callers should treat nil as a no-op.
func New(ctx context.Context) (*Client, error) {
	from := os.Getenv("EMAIL_FROM")
	if from == "" {
		return nil, nil
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("email: load AWS config: %w", err)
	}

	return &Client{
		ses:  sesv2.NewFromConfig(cfg),
		from: from,
	}, nil
}

// Message holds the fields for a single transactional email.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string // plain-text fallback
}

// Send delivers a single email. If the Client is nil (EMAIL_FROM unset), it is a no-op.
func (c *Client) Send(ctx context.Context, m Message) error {
	if c == nil {
		return nil
	}

	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(c.from),
		Destination: &types.Destination{
			ToAddresses: []string{m.To},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    aws.String(m.Subject),
					Charset: aws.String("UTF-8"),
				},
				Body: &types.Body{
					Html: &types.Content{
						Data:    aws.String(m.HTML),
						Charset: aws.String("UTF-8"),
					},
					Text: &types.Content{
						Data:    aws.String(m.Text),
						Charset: aws.String("UTF-8"),
					},
				},
			},
		},
	}

	_, err := c.ses.SendEmail(ctx, input)
	return err
}

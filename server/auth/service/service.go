// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/server/auth/internal"
	"github.com/luci/luci-go/server/auth/service/protocol"
	"github.com/luci/luci-go/server/auth/signing"
)

// Notification represents a notification about AuthDB change. Must be acked
// once processed.
type Notification struct {
	Revision int64 // new auth DB revision

	service      *AuthService // parent service
	subscription string       // subscription name the change was pulled from
	ackIDs       []string     // IDs of all messages to ack
}

// Acknowledge tells PubSub to stop redelivering this notification.
func (n *Notification) Acknowledge(c context.Context) error {
	return n.service.ackMessages(c, n.subscription, n.ackIDs)
}

// pubSubMessage is received via PubSub when AuthDB changes.
type pubSubMessage struct {
	AckID   string `json:"ackId"` // set only when pulling
	Message struct {
		MessageID  string            `json:"messageId"`
		Data       string            `json:"data"`
		Attributes map[string]string `json:"attributes"`
	} `json:"message"`
}

// AuthService represents API exposed by auth_service. All methods expect
// an authenticating transport to be installed into the context (see
// github.com/luci/luci-go/common/transport).
type AuthService struct {
	// URL is URL (with protocol) of auth_service (e.g. "https://<host>").
	URL string

	// pubSubURLRoot is root URL of PubSub service. Mocked in tests.
	pubSubURLRoot string
}

// pubSubURL returns full URL to pubsub API endpoint.
func (s *AuthService) pubSubURL(path string) string {
	if s.pubSubURLRoot != "" {
		return s.pubSubURLRoot + path
	}
	return "https://pubsub.googleapis.com/v1/" + path
}

// EnsureSubscription creates a new subscription to AuthDB change notifications
// topic or changes its pushURL if it already exists. `subscription` is full
// subscription name e.g "projects/<projectid>/subscriptions/<subid>". Name of
// the topic is fetched from the auth service. Returns nil if such subscription
// already exists.
func (s *AuthService) EnsureSubscription(c context.Context, subscription, pushURL string) error {
	// Subscription already exists?
	var existing struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
		PushConfig struct {
			PushEndpoint string `json:"pushEndpoint"`
		} `json:"pushConfig"`
	}
	err := internal.GetJSON(c, s.pubSubURL(subscription), &existing)
	if err != nil && existing.Error.Code != 404 {
		return err
	}

	// Create a new subscription if existing is missing.
	if err != nil {
		// Make sure caller has access.
		var response struct {
			Topic string `json:"topic"`
		}
		url := s.URL + "/auth_service/api/v1/authdb/subscription/authorization"
		if err := internal.PostJSON(c, url, nil, &response); err != nil {
			return err
		}
		topic := response.Topic

		// Create the subscription.
		var config struct {
			Topic      string `json:"topic"`
			PushConfig struct {
				PushEndpoint string `json:"pushEndpoint"`
			} `json:"pushConfig"`
			AckDeadlineSeconds int `json:"ackDeadlineSeconds"`
		}
		config.Topic = topic
		config.PushConfig.PushEndpoint = pushURL
		config.AckDeadlineSeconds = 60
		return internal.PutJSON(c, s.pubSubURL(subscription), &config, nil)
	}

	// Is existing subscription configured correctly already?
	if existing.PushConfig.PushEndpoint == pushURL {
		return nil
	}

	// Reconfigure existing subscription.
	var request struct {
		PushConfig struct {
			PushEndpoint string `json:"pushEndpoint"`
		} `json:"pushConfig"`
	}
	request.PushConfig.PushEndpoint = pushURL
	return internal.PostJSON(c, s.pubSubURL(subscription+":modifyPushConfig"), &request, nil)
}

// PullPubSub pulls pending PubSub messages (from subscription created
// previously by EnsureSubscription), authenticates them, and converts
// them into Notification object. Returns (nil, nil) if no pending messages.
// Does not wait for messages to arrive.
func (s *AuthService) PullPubSub(c context.Context, subscription string) (*Notification, error) {
	request := struct {
		ReturnImmediately bool `json:"returnImmediately"`
		MaxMessages       int  `json:"maxMessages"`
	}{true, 1000}

	response := struct {
		ReceivedMessages []pubSubMessage `json:"receivedMessages"`
	}{}

	url := s.pubSubURL(subscription + ":pull")
	if err := internal.PostJSON(c, url, &request, &response); err != nil {
		return nil, err
	}
	if len(response.ReceivedMessages) == 0 {
		return nil, nil
	}

	logging.Infof(c, "Received PubSub notification: %d", len(response.ReceivedMessages))

	// Grab certs to verify signatures on PubSub messages.
	certs, err := signing.FetchCertificates(c, s.URL+"/auth/api/v1/server/certificates")
	if err != nil {
		return nil, err
	}

	// Find maximum AuthDB revision among all verified messages. Invalid messages
	// are skipped (with logging).
	maxRev := int64(-1)
	ackIDs := ([]string)(nil)
	for _, msg := range response.ReceivedMessages {
		if msg.AckID != "" {
			ackIDs = append(ackIDs, msg.AckID)
		}
		notify, err := s.decodeMessage(&msg, certs)
		if err != nil {
			logging.Warningf(c, "Bad signature on message %s - %s", msg.Message.MessageID, err)
			continue
		}
		if notify.GetRevision().GetAuthDbRev() > maxRev {
			maxRev = notify.GetRevision().GetAuthDbRev()
		}
	}

	// All message are invalid and skipped? Try to acknowledge them to remove
	// them from the queue, but don't error if it fails.
	if maxRev == -1 {
		err := s.ackMessages(c, subscription, ackIDs)
		if err != nil {
			logging.Warningf(c, "Failed to ack some PubSub messages (%v) - %s", ackIDs, err)
		}
		return nil, nil
	}

	logging.Infof(c, "Auth service notifies us that the most recent AuthDB revision is %d", maxRev)
	return &Notification{
		Revision:     maxRev,
		service:      s,
		subscription: subscription,
		ackIDs:       ackIDs,
	}, nil
}

// ProcessPubSubPush handles incoming PubSub push notification. `body` is
// the entire body of the push HTTP request. Invalid messages are silently
// skipped by returning nil error (to avoid redelivery). The error is still
// logged though.
func (s *AuthService) ProcessPubSubPush(c context.Context, body []byte) (*Notification, error) {
	msg := pubSubMessage{}
	if err := json.Unmarshal(body, &msg); err != nil {
		logging.Errorf(c, "auth: bad PubSub notification, not JSON - %s", err)
		return nil, nil
	}

	// It's fine to return error here. Certificate fetch can fail due to bad
	// connectivity and we need a retry.
	certs, err := signing.FetchCertificates(c, s.URL+"/auth/api/v1/server/certificates")
	if err != nil {
		return nil, err
	}

	notify, err := s.decodeMessage(&msg, certs)
	if err != nil {
		logging.Errorf(c, "auth: bad PubSub notification - %s", err)
		return nil, nil
	}

	return &Notification{
		Revision: notify.GetRevision().GetAuthDbRev(),
		service:  s,
	}, nil
}

///

// decodeMessage checks that the message payload was signed by Auth service key
// and deserializes it.
func (s *AuthService) decodeMessage(m *pubSubMessage, certs *signing.PublicCertificates) (*protocol.ChangeNotification, error) {
	// Key name used to sign.
	keyName := m.Message.Attributes["X-AuthDB-SigKey-v1"]
	if keyName == "" {
		return nil, fmt.Errorf("X-AuthDB-SigKey-v1 attribute is not set")
	}

	// The signature.
	sigBase64 := m.Message.Attributes["X-AuthDB-SigVal-v1"]
	if sigBase64 == "" {
		return nil, fmt.Errorf("X-AuthDB-SigVal-v1 attribute is not set")
	}
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return nil, fmt.Errorf("the message signature is not valid base64 - %s", err)
	}

	// Message body.
	body, err := base64.StdEncoding.DecodeString(m.Message.Data)
	if err != nil {
		return nil, fmt.Errorf("the message body is not valid base64 - %s", err)
	}

	// Validate.
	if err := certs.CheckSignature(keyName, body, sig); err != nil {
		return nil, err
	}

	// Deserialize.
	out := protocol.ChangeNotification{}
	if err := proto.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ackMessages acknowledges processing of pubsub messages.
func (s *AuthService) ackMessages(c context.Context, subscription string, ackIDs []string) error {
	if len(ackIDs) == 0 {
		return nil
	}
	url := s.pubSubURL(subscription + ":acknowledge")
	request := struct {
		AckIDs []string `json:"ackIds"`
	}{ackIDs}
	return internal.PostJSON(c, url, &request, nil)
}

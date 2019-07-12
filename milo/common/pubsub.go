// Copyright 2019 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/pubsub"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/api/config"
)

var pubsubClientFactoryKey = "stores a pubsubClientFactory"

type PubSubMessage struct {
	Attributes map[string]interface{} `json:"attributes"`
	Data       string                 `json:"data"`
	MessageID  string                 `json:"message_id"`
}

type PubSubSubscription struct {
	Message      PubSubMessage `json:"message"`
	Subscription string        `json:"subscription"`
}

var errNotExist = errors.New("does not exist")

// GetData returns the expanded form of Data (decoded from base64).
func (m *PubSubSubscription) GetData() ([]byte, error) {
	return base64.StdEncoding.DecodeString(m.Message.Data)
}

// pubsubClient is an interface representing a pubsub.Client containing only
// the functions that Milo calls.  Internal use only, can be swapped
// out for testing.
type pubsubClient interface {
	// close releases any resources held by the client, such as memory and
	// goroutines.
	//
	// Uses the context for logging.
	close(context.Context)

	// getTopic returns the pubsub topic if it exists, a notExist error if
	// it does not exist, or an error if there was an error.
	getTopic(context.Context, string) (*pubsub.Topic, error)

	// getSubscription returns the pubsub subscription if it exists,
	// a notExist error if it does not exist, or an error if there was an error.
	getSubscription(context.Context, string) (*pubsub.Subscription, error)

	createSubscription(context.Context, string, pubsub.SubscriptionConfig) (
		*pubsub.Subscription, error)
}

// pubsubClientFactory is a stubbable factory that produces pubsubClients bound
// to project IDs.
type pubsubClientFactory func(context.Context, string) (pubsubClient, error)

// prodPubSubClient is a wrapper around the production pubsub client.
type prodPubSubClient struct {
	*pubsub.Client
}

func (pc *prodPubSubClient) close(c context.Context) {
	if err := pc.Close(); err != nil {
		logging.WithError(err).Errorf(c, "Failed to close PubSub client")
	}
}

func (pc *prodPubSubClient) getTopic(c context.Context, id string) (*pubsub.Topic, error) {
	topic := pc.Client.Topic(id)
	exists, err := topic.Exists(c)
	switch {
	case err != nil:
		return nil, err
	case !exists:
		return nil, errNotExist
	}
	return topic, nil
}

func (pc *prodPubSubClient) getSubscription(c context.Context, id string) (*pubsub.Subscription, error) {
	sub := pc.Client.Subscription(id)
	exists, err := sub.Exists(c)
	switch {
	case err != nil:
		return nil, err
	case !exists:
		return nil, errNotExist
	}
	return sub, nil
}

func (pc *prodPubSubClient) createSubscription(
	c context.Context, id string, cfg pubsub.SubscriptionConfig) (
	*pubsub.Subscription, error) {

	return pc.Client.CreateSubscription(c, id, cfg)
}

func prodPubSubClientFactory(c context.Context, projectID string) (pubsubClient, error) {
	cli, err := pubsub.NewClient(c, projectID)
	return &prodPubSubClient{cli}, err
}

// withClientFactory returns a context with a given pubsub client factory.
func withClientFactory(c context.Context, fac pubsubClientFactory) context.Context {
	return context.WithValue(c, &pubsubClientFactoryKey, fac)
}

func newPubSubClient(c context.Context, projectID string) (pubsubClient, error) {
	if fac, ok := c.Value(&pubsubClientFactoryKey).(pubsubClientFactory); !ok {
		panic("no pubsub client factory installed")
	} else {
		return fac(c, projectID)
	}
}

// EnsurePubSubSubscribed makes sure the following subscriptions are in place:
// * buildbucket, via the settings.Buildbucket.Topic setting
func EnsurePubSubSubscribed(c context.Context, settings *config.Settings) error {
	if settings.Buildbucket != nil {
		c = withClientFactory(c, prodPubSubClientFactory)
		return ensureBuildbucketSubscribed(c, settings.Buildbucket.Project)
	}
	// TODO(hinoka): Ensure buildbot subscribed.
	return nil
}

// ensureBuildbucketSubscribedis called by a cron job and ensures that the Milo
// instance is properly subscribed to the buildbucket subscription endpoint.
func ensureBuildbucketSubscribed(c context.Context, projectID string) error {
	topicID := "builds"

	// Check the buildbucket project to see if the topic exists first.
	bbClient, err := newPubSubClient(c, projectID)
	if err != nil {
		return err
	}
	defer bbClient.close(c)

	topic, err := bbClient.getTopic(c, topicID)
	switch err {
	case errNotExist:
		return errors.Annotate(err, "%s does not exist", topicID).Err()
	case nil:
		// continue
	default:
		if strings.Contains(err.Error(), "PermissionDenied") {
			URL := "https://console.cloud.google.com/iam-admin/iam/project?project=" + projectID
			acct, serr := info.ServiceAccount(c)
			if serr != nil {
				acct = fmt.Sprintf("Unknown: %s", serr.Error())
			}
			// The documentation is incorrect.  We need Editor permission because
			// the Subscriber permission does NOT permit attaching subscriptions to
			// topics or to view topics.
			logging.WithError(err).Errorf(
				c, "please go to %s and add %s as a Pub/Sub Editor", URL, acct)
		} else {
			logging.WithError(err).Errorf(c, "could not check topic %#v", topic)
		}
		return err
	}

	// Now check to see if the subscription already exists.
	miloClient, err := newPubSubClient(c, info.AppID(c))
	if err != nil {
		return err
	}
	defer miloClient.close(c)

	sub, err := miloClient.getSubscription(c, "buildbucket")
	switch err {
	case errNotExist:
		// continue
	case nil:
		logging.Infof(c, "subscription %#v exists, no need to update", sub)
		return nil
	default:
		logging.WithError(err).Errorf(c, "could not check subscription %#v", sub)
		return err
	}
	// Get the pubsub module of our app.  We do not want to use info.ModuleHostname()
	// because it returns a version pinned hostname instead of the default route.
	pubsubModuleHost := "pubsub." + info.DefaultVersionHostname(c)

	// No subscription exists, attach a new subscription to the existing topic.
	endpointURL := url.URL{
		Scheme: "https",
		Host:   pubsubModuleHost,
		Path:   "/_ah/push-handlers/buildbucket",
	}
	subConfig := pubsub.SubscriptionConfig{
		Topic:       topic,
		PushConfig:  pubsub.PushConfig{Endpoint: endpointURL.String()},
		AckDeadline: time.Minute * 10,
	}
	newSub, err := miloClient.createSubscription(c, "buildbucket", subConfig)
	if err != nil {
		return errors.Annotate(err, "could not create subscription %#v", sub).Err()
	}
	// Success!
	logging.Infof(c, "successfully created subscription %#v", newSub)
	return nil
}

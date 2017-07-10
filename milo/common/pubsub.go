package common

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/milo/api/config"
)

var pubSubClientKey = "stores a pubsubClient"

type PubSubMessage struct {
	Attributes map[string]string `json:"attributes"`
	Data       string            `json:"data"`
	MessageID  string            `json:"message_id"`
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
	// getTopic returns the pubsub topic if it exists, a notExist error if
	// it does not exist, or an error if there was an error.
	getTopic(string) (*pubsub.Topic, error)

	// getSubscription returns the pubsub subscription if it exists,
	// a notExist error if it does not exist, or an error if there was an error.
	getSubscription(string) (*pubsub.Subscription, error)

	createSubscription(string, pubsub.SubscriptionConfig) (
		*pubsub.Subscription, error)
}

// prodPubSubClient is a wrapper around the production pubsub client.
type prodPubSubClient struct {
	ctx    context.Context
	client *pubsub.Client
}

func (pc *prodPubSubClient) getTopic(id string) (*pubsub.Topic, error) {
	topic := pc.client.Topic(id)
	exists, err := topic.Exists(pc.ctx)
	switch {
	case err != nil:
		return nil, err
	case !exists:
		return nil, errNotExist
	}
	return topic, nil
}

func (pc *prodPubSubClient) getSubscription(id string) (*pubsub.Subscription, error) {
	sub := pc.client.Subscription(id)
	exists, err := sub.Exists(pc.ctx)
	switch {
	case err != nil:
		return nil, err
	case !exists:
		return nil, errNotExist
	}
	return sub, nil
}

func (pc *prodPubSubClient) createSubscription(id string, cfg pubsub.SubscriptionConfig) (
	*pubsub.Subscription, error) {

	return pc.client.CreateSubscription(pc.ctx, id, cfg)
}

// getPubSubClient extracts a debug PubSub client out of the context.
func getPubSubClient(c context.Context) (pubsubClient, error) {
	if client, ok := c.Value(&pubSubClientKey).(pubsubClient); ok {
		return client, nil
	}
	return nil, errors.New("no pubsub clients installed")
}

// withClient returns a context with a pubsub client instantiated to the
// given project ID
func withClient(c context.Context, projectID string) (context.Context, error) {
	if projectID == "" {
		return nil, errors.New("missing buildbucket project")
	}
	client, err := pubsub.NewClient(c, projectID)
	if err != nil {
		return nil, err
	}
	return context.WithValue(c, &pubSubClientKey, &prodPubSubClient{c, client}), nil
}

func getTopic(c context.Context, id string) (*pubsub.Topic, error) {
	client, err := getPubSubClient(c)
	if err != nil {
		return nil, err
	}
	return client.getTopic(id)
}

func getSubscription(c context.Context, id string) (*pubsub.Subscription, error) {
	client, err := getPubSubClient(c)
	if err != nil {
		return nil, err
	}
	return client.getSubscription(id)
}

func createSubscription(c context.Context, id string, cfg pubsub.SubscriptionConfig) (
	*pubsub.Subscription, error) {

	client, err := getPubSubClient(c)
	if err != nil {
		return nil, err
	}
	return client.createSubscription(id, cfg)
}

// EnsurePubSubSubscribed makes sure the following subscriptions are in place:
// * buildbucket, via the settings.Buildbucket.Topic setting
func EnsurePubSubSubscribed(c context.Context, settings *config.Settings) error {
	if settings.Buildbucket != nil {
		// Install the production pubsub client pointing to the buildbucket project
		// into the context.
		c, err := withClient(c, settings.Buildbucket.Project)
		if err != nil {
			return err
		}
		return ensureBuildbucketSubscribed(c, settings.Buildbucket.Project)
	}
	// TODO(hinoka): Ensure buildbot subscribed.
	return nil
}

// ensureSubscribed is called by a cron job and ensures that the Milo
// instance is properly subscribed to the buildbucket subscription endpoint.
func ensureBuildbucketSubscribed(c context.Context, projectID string) error {
	topicID := "builds"
	// Check to see if the topic exists first.
	topic, err := getTopic(c, topicID)
	switch err {
	case errNotExist:
		logging.WithError(err).Errorf(c, "%s does not exist", topicID)
		return err
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
	subID := info.AppID(c)
	// Get the pubsub module of our app.  We do not want to use info.ModuleHostname()
	// because it returns a version pinned hostname instead of the default route.
	sub, err := getSubscription(c, subID)
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
	newSub, err := createSubscription(c, subID, subConfig)
	if err != nil {
		if strings.Contains(err.Error(), "The supplied HTTP URL is not registered") {
			registerURL := "https://console.cloud.google.com/apis/credentials/domainverification?project=" + projectID
			verifyURL := "https://www.google.com/webmasters/verification/verification?hl=en-GB&siteUrl=http://" + pubsubModuleHost
			logging.WithError(err).Errorf(
				c, "The domain has to be verified and added.\n\n"+
					"1. Go to %s\n"+
					"2. Verify the domain\n"+
					"3. Go to %s\n"+
					"4. Add %s to allowed domains\n\n",
				verifyURL, registerURL, pubsubModuleHost)
		} else {
			logging.WithError(err).Errorf(c, "could not create subscription %#v", sub)
		}
		return err
	}
	// Success!
	logging.Infof(c, "successfully created subscription %#v", newSub)
	return nil
}

// Copyright 2016 The LUCI Authors.
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

package logdog

import (
	"fmt"
	"runtime"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	ps "go.chromium.org/luci/common/gcloud/pubsub"
	"go.chromium.org/luci/common/lhttp"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	api "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	"go.chromium.org/luci/logdog/client/butler/output"
	out "go.chromium.org/luci/logdog/client/butler/output/pubsub"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/luci_config/common/cfgtypes"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
	"google.golang.org/api/option"
)

// Scopes returns the set of OAuth scopes required for this Output.
func Scopes() []string {
	// E-mail scope needed for Coordinator authentication.
	scopes := []string{auth.OAuthScopeEmail}
	// Publisher scope needed to publish to Pub/Sub transport.
	scopes = append(scopes, ps.PublisherScopes...)

	return scopes
}

// Config is the set of configuration parameters for this Output instance.
type Config struct {
	// Auth is the Authenticator to use for registration and publishing. It should
	// be configured to hold the scopes returned by Scopes.
	Auth *auth.Authenticator

	// Host is the name of the LogDog Host to connect to.
	Host string

	// Project is the project that this stream belongs to.
	Project cfgtypes.ProjectName
	// Prefix is the stream prefix to register.
	Prefix types.StreamName
	// PrefixExpiration is the prefix expiration to use when registering.
	// If zero, no expiration will be expressed to the Coordinator, and it will
	// choose based on its configuration.
	PrefixExpiration time.Duration

	// SourceInfo, if not empty, is auxiliary source information to register
	// alongside the stream.
	SourceInfo []string

	// PublishContext is the special Context to use for publishing messages. If
	// nil, the Context supplied to Register will be used.
	//
	// This is useful when the Context supplied to Register responds to
	// cancellation (e.g., user sends SIGTERM), but we might not want to
	// immediately cancel pending publishes due to flushing.
	PublishContext context.Context

	// RPCTimeout, if > 0, is the timeout to apply to an individual RPC.
	RPCTimeout time.Duration

	// Track, if true, instructs this Output instance to track all log entries
	// that have been sent in-memory. This is useful for debugging.
	Track bool
}

// Register registers the supplied Prefix with the Coordinator. Upon success,
// an Output instance bound to that stream will be returned.
func (cfg *Config) Register(c context.Context) (output.Output, error) {
	// Validate our configuration parameters.
	switch {
	case cfg.Auth == nil:
		return nil, errors.New("no authenticator supplied")
	case cfg.Host == "":
		return nil, errors.New("no host supplied")
	}
	if err := cfg.Project.Validate(); err != nil {
		return nil, errors.Annotate(err, "failed to validate project").
			InternalReason("project(%v)", cfg.Project).Err()
	}
	if err := cfg.Prefix.Validate(); err != nil {
		return nil, errors.Annotate(err, "failed to validate prefix").
			InternalReason("prefix(%v)", cfg.Prefix).Err()
	}

	// Open a pRPC client to our Coordinator instance.
	httpClient, err := cfg.Auth.Client()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get authenticated HTTP client.")
		return nil, err
	}

	// Configure our pRPC client.
	clientOpts := prpc.DefaultOptions()
	clientOpts.PerRPCTimeout = cfg.RPCTimeout
	client := prpc.Client{
		C:       httpClient,
		Host:    cfg.Host,
		Options: clientOpts,
	}

	// If our host begins with "localhost", set insecure option automatically.
	if lhttp.IsLocalHost(cfg.Host) {
		log.Infof(c, "Detected localhost; enabling insecure RPC connection.")
		client.Options.Insecure = true
	}

	// Register our Prefix with the Coordinator.
	log.Fields{
		"prefix": cfg.Prefix,
		"host":   cfg.Host,
	}.Debugf(c, "Registering prefix space with Coordinator service.")

	// Build our source info.
	sourceInfo := make([]string, 0, len(cfg.SourceInfo)+2)
	sourceInfo = append(sourceInfo, cfg.SourceInfo...)
	sourceInfo = append(sourceInfo,
		fmt.Sprintf("GOARCH=%s", runtime.GOARCH),
		fmt.Sprintf("GOOS=%s", runtime.GOOS),
	)

	nonce := make([]byte, types.OpNonceLength)
	if _, err = cryptorand.Read(c, nonce); err != nil {
		log.WithError(err).Errorf(c, "Failed to generate RegisterPrefix nonce.")
		return nil, errors.Annotate(err, "generating nonce").Err()
	}
	req := &api.RegisterPrefixRequest{
		Project:    string(cfg.Project),
		Prefix:     string(cfg.Prefix),
		SourceInfo: sourceInfo,
		Expiration: google.NewDuration(cfg.PrefixExpiration),
		OpNonce:    nonce,
	}

	svc := api.NewRegistrationPRPCClient(&client)
	var resp *api.RegisterPrefixResponse
	err = retry.Retry(c, retry.Default, func() error {
		var err error
		resp, err = svc.RegisterPrefix(c, req)
		return grpcutil.WrapIfTransient(err)
	}, retry.LogCallback(c, "RegisterPrefix"))
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to register prefix with Coordinator service.")
		return nil, err
	}
	log.Fields{
		"prefix":      cfg.Prefix,
		"bundleTopic": resp.LogBundleTopic,
	}.Debugf(c, "Successfully registered log stream prefix.")

	// Validate the response topic.
	fullTopic := ps.Topic(resp.LogBundleTopic)
	if err := fullTopic.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"fullTopic":  fullTopic,
		}.Errorf(c, "Coordinator returned invalid Pub/Sub topic.")
		return nil, err
	}

	// Split our topic into project and topic name. This must succeed, since we
	// just finished validating the topic.
	proj, topic := fullTopic.Split()

	// Instantiate our Pub/Sub instance.
	//
	// We will use the non-cancelling context, for all Pub/Sub calls, as we want
	// the Pub/Sub system to drain without interruption if the application is
	// otherwise canceled.
	pctx := cfg.PublishContext
	if pctx == nil {
		pctx = c
	}

	tokenSource, err := cfg.Auth.TokenSource()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get TokenSource for Pub/Sub client.")
		return nil, err
	}

	psClient, err := pubsub.NewClient(pctx, proj, option.WithTokenSource(tokenSource))
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    proj,
		}.Errorf(c, "Failed to create Pub/Sub client.")
		return nil, errors.New("failed to get Pub/Sub client")
	}
	psTopic := psClient.Topic(topic)

	// Assert that our Topic exists.
	exists, err := retryTopicExists(c, psTopic, cfg.RPCTimeout)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    proj,
			"topic":      topic,
		}.Errorf(c, "Failed to check for Pub/Sub topic.")
		return nil, errors.New("failed to check for Pub/Sub topic")
	}
	if !exists {
		log.Fields{
			"fullTopic": fullTopic,
		}.Errorf(c, "Pub/Sub Topic does not exist.")
		return nil, errors.New("PubSub topic does not exist")
	}

	// We own the prefix and all verifiable parameters have been validated.
	// Successfully return our Output instance.
	//
	// Note that we use our publishing context here.
	return out.New(pctx, out.Config{
		Topic:      pubSubTopicWrapper{psTopic},
		Secret:     resp.Secret,
		Compress:   true,
		Track:      cfg.Track,
		RPCTimeout: cfg.RPCTimeout,
	}), nil
}

func retryTopicExists(ctx context.Context, t *pubsub.Topic, rpcTimeout time.Duration) (bool, error) {
	var exists bool
	err := retry.Retry(ctx, retry.Default, func() (err error) {
		ctx := ctx
		if rpcTimeout > 0 {
			var cancelFunc context.CancelFunc
			ctx, cancelFunc = clock.WithTimeout(ctx, rpcTimeout)
			defer cancelFunc()
		}

		exists, err = t.Exists(ctx)
		return
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
		}.Errorf(ctx, "Failed to check if topic exists; retrying...")
	})
	return exists, err
}

// pubSubTopicWrapper wraps a cloud pubsub package Topic and converts it into
// a Butler pubsub.Topic.
type pubSubTopicWrapper struct {
	t *pubsub.Topic
}

func (w pubSubTopicWrapper) String() string {
	return w.t.String()
}

func (w pubSubTopicWrapper) Publish(ctx context.Context, msg *pubsub.Message) (string, error) {
	return w.t.Publish(ctx, msg).Get(ctx)
}

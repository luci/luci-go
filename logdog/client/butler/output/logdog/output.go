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
	"context"
	"fmt"
	"runtime"
	"time"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	ps "go.chromium.org/luci/common/gcloud/pubsub"
	"go.chromium.org/luci/common/lhttp"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"

	api "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	"go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/logdog/common/types"
)

// Scopes returns the set of OAuth scopes required for this Output.
func Scopes() []string {
	// E-mail scope needed for Coordinator authentication.
	scopes := scopes.DefaultScopeSet()
	// Publisher scope needed to publish to Pub/Sub transport.
	scopes = append(scopes, ps.PublisherScopes...)

	return scopes
}

// Config is the set of configuration parameters for this Output instance.
type Config struct {
	// Auth incapsulates an authentication scheme to use.
	//
	// Construct it using either LegacyAuth() or RealmsAwareAuth().
	Auth Auth

	// Host is the name of the LogDog Host to connect to.
	Host string

	// Project is the project that this stream belongs to.
	Project string
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
}

// Register registers the supplied Prefix with the Coordinator. Upon success,
// an Output instance bound to that stream will be returned.
func (cfg *Config) Register(c context.Context) (output.Output, error) {
	// Validate our configuration parameters.
	switch {
	case cfg.Auth == nil:
		return nil, errors.New("no auth provider supplied")
	case cfg.Host == "":
		return nil, errors.New("no host supplied")
	}
	if err := config.ValidateProjectName(cfg.Project); err != nil {
		return nil, errors.Fmt("failed to validate project %q: %w", cfg.Project, err)
	}
	if err := cfg.Prefix.Validate(); err != nil {
		return nil, errors.Fmt("failed to validate prefix %q: %w", cfg.Prefix, err)
	}

	// Check no cross-project shenanigans occur. Eventually cfg.Project will be
	// removed in favor of providing the realm name.
	if proj := cfg.Auth.Project(); proj != "" && proj != cfg.Project {
		return nil, errors.Fmt("registering a prefix in project %q while running in a context of project %q is forbidden",
			cfg.Project, proj)
	}

	// TODO(crbug.com/1172492): When running in "realms mode" Auth.RPC should be
	// used to register the prefix. But it may fail for misconfigured projects
	// that still expect the system account for the registration RPC, so we
	// fallback to Auth.PubSub in this case. Once there are no HTTP 403 errors in
	// the server logs, this fallback can be removed.
	resp, err := cfg.registerPrefix(c, cfg.Auth.RPC())
	if cfg.Auth.Realm() != "" && status.Code(err) == codes.PermissionDenied {
		log.Errorf(c, "Failed to register the prefix using the default account, trying the system one as a fallback")
		resp, err = cfg.registerPrefix(c, cfg.Auth.PubSub())
	}
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

	tokenSource, err := cfg.Auth.PubSub().TokenSource()
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

	// We own the prefix and all verifiable parameters have been validated.
	// Successfully return our Output instance.
	//
	// Note that we use our publishing context here.
	return newPubsub(pctx, pubsubConfig{
		Topic:      pubSubTopicWrapper{psTopic},
		Host:       cfg.Host,
		Project:    cfg.Project,
		Prefix:     string(cfg.Prefix),
		Secret:     resp.Secret,
		Compress:   true,
		RPCTimeout: cfg.RPCTimeout,
	}), nil
}

func (cfg *Config) registerPrefix(c context.Context, auth *auth.Authenticator) (*api.RegisterPrefixResponse, error) {
	// Open a pRPC client to our Coordinator instance.
	httpClient, err := auth.Client()
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
		return nil, errors.Fmt("generating nonce: %w", err)
	}
	req := &api.RegisterPrefixRequest{
		Project:    string(cfg.Project),
		Realm:      cfg.Auth.Realm(),
		Prefix:     string(cfg.Prefix),
		SourceInfo: sourceInfo,
		Expiration: durationpb.New(cfg.PrefixExpiration),
		OpNonce:    nonce,
	}

	svc := api.NewRegistrationPRPCClient(&client)
	var resp *api.RegisterPrefixResponse
	err = retry.Retry(c, transient.Only(retry.Default), func() error {
		var err error
		resp, err = svc.RegisterPrefix(c, req)
		return grpcutil.WrapIfTransient(err)
	}, retry.LogCallback(c, "RegisterPrefix"))
	return resp, err
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

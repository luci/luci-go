// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/luci/luci-go/client/internal/logdog/butler/output"
	out "github.com/luci/luci-go/client/internal/logdog/butler/output/pubsub"
	api "github.com/luci/luci-go/common/api/logdog_coordinator/registration/v1"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock/clockflag"
	"github.com/luci/luci-go/common/flag/multiflag"
	ps "github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/prpc"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	"google.golang.org/cloud/pubsub"
)

func init() {
	registerOutputFactory(new(logdogOutputFactory))
}

// logdogOutputFactory for publishing logs using a LogDog Coordinator host.
type logdogOutputFactory struct {
	host             string
	prefixExpiration clockflag.Duration

	track bool
}

var _ outputFactory = (*logdogOutputFactory)(nil)

func (f *logdogOutputFactory) option() multiflag.Option {
	opt := newOutputOption("logdog", "Output to a LogDog Coordinator instance.", f)

	flags := opt.Flags()
	flags.StringVar(&f.host, "host", "",
		"The LogDog Coordinator host name.")
	flags.Var(&f.prefixExpiration, "prefix-expiration",
		"Amount of time after registration that the prefix will be active. If omitted, the service "+
			"default will be used. This should exceed the expected lifetime of the job by a fair margin.")

	// TODO(dnj): Default to false when mandatory debugging is finished.
	flags.BoolVar(&f.track, "track", true,
		"Track each sent message and dump at the end. This adds CPU/memory overhead.")

	return opt
}

func (f *logdogOutputFactory) configOutput(a *application) (output.Output, error) {
	// Open a pRPC client to our Coordinator instance.
	authenticator, err := a.authenticator(a)
	if err != nil {
		log.WithError(err).Errorf(a, "Failed to get authenticator.")
		return nil, err
	}
	httpClient, err := authenticator.Client()
	if err != nil {
		log.WithError(err).Errorf(a, "Failed to get authenticated HTTP client.")
		return nil, err
	}

	// Configure our pRPC client.
	client := prpc.Client{
		C:       httpClient,
		Host:    f.host,
		Options: prpc.DefaultOptions(),
	}

	// If our host begins with "localhost", set insecure option automatically.
	if isLocalHost(f.host) {
		log.Infof(a, "Detected localhost; enabling insecure RPC connection.")
		client.Options.Insecure = true
	}

	// Register our Prefix with the Coordinator.
	log.Fields{
		"prefix": a.prefix,
		"host":   f.host,
	}.Debugf(a, "Registering prefix space with Coordinator service.")

	svc := api.NewRegistrationPRPCClient(&client)
	resp, err := svc.RegisterPrefix(a, &api.RegisterPrefixRequest{
		Project: string(a.project),
		Prefix:  string(a.prefix),
		SourceInfo: []string{
			"LogDog Butler",
			fmt.Sprintf("GOARCH=%s", runtime.GOARCH),
			fmt.Sprintf("GOOS=%s", runtime.GOOS),
		},
		Expiration: google.NewDuration(time.Duration(f.prefixExpiration)),
	})
	if err != nil {
		log.WithError(err).Errorf(a, "Failed to register prefix with Coordinator service.")
		return nil, err
	}
	log.Fields{
		"prefix":      a.prefix,
		"bundleTopic": resp.LogBundleTopic,
	}.Debugf(a, "Successfully registered log stream prefix.")

	// Validate the response topic.
	fullTopic := ps.Topic(resp.LogBundleTopic)
	if err := fullTopic.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"fullTopic":  fullTopic,
		}.Errorf(a, "Coordinator returned invalid Pub/Sub topic.")
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
	psClient, err := pubsub.NewClient(a.ncCtx, proj, cloud.WithTokenSource(authenticator.TokenSource()))
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    proj,
		}.Errorf(a, "Failed to create Pub/Sub client.")
		return nil, errors.New("failed to get Pub/Sub client")
	}
	psTopic := psClient.Topic(topic)

	// Assert that our Topic exists.
	exists, err := retryTopicExists(a, psTopic)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    proj,
			"topic":      topic,
		}.Errorf(a, "Failed to check for Pub/Sub topic.")
		return nil, errors.New("failed to check for Pub/Sub topic")
	}
	if !exists {
		log.Fields{
			"fullTopic": fullTopic,
		}.Errorf(a, "Pub/Sub Topic does not exist.")
		return nil, errors.New("PubSub topic does not exist")
	}

	// We own the prefix and all verifiable parameters have been validated.
	// Successfully return our Output instance.
	//
	// Note that we use our non-cancelling context here.
	return out.New(a.ncCtx, out.Config{
		Topic:    psTopic,
		Secret:   resp.Secret,
		Compress: true,
		Track:    f.track,
	}), nil
}

func (f *logdogOutputFactory) scopes() []string {
	// E-mail scope needed for Coordinator authentication.
	scopes := []string{auth.OAuthScopeEmail}
	// Publisher scope needed to publish to Pub/Sub transport.
	scopes = append(scopes, ps.PublisherScopes...)
	return scopes
}

func retryTopicExists(ctx context.Context, t *pubsub.Topic) (bool, error) {
	var exists bool
	err := retry.Retry(ctx, retry.Default, func() (err error) {
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

func isLocalHost(host string) bool {
	switch {
	case host == "localhost", strings.HasPrefix(host, "localhost:"):
	case host == "127.0.0.1", strings.HasPrefix(host, "127.0.0.1:"):
	case host == "[::1]", strings.HasPrefix(host, "[::1]:"):
	case strings.HasPrefix(host, ":"):

	default:
		return false
	}
	return true
}

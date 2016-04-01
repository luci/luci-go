// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"time"

	"github.com/luci/luci-go/client/internal/logdog/butler/output"
	out "github.com/luci/luci-go/client/internal/logdog/butler/output/pubsub"
	"github.com/luci/luci-go/common/flag/multiflag"
	ps "github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	"google.golang.org/cloud/pubsub"
)

func init() {
	registerOutputFactory(new(pubsubOutputFactory))
}

// pubsubOutputFactory for Google Cloud PubSub.
type pubsubOutputFactory struct {
	topic      ps.Topic
	noCompress bool
	track      bool
}

var _ outputFactory = (*pubsubOutputFactory)(nil)

func (f *pubsubOutputFactory) option() multiflag.Option {
	opt := newOutputOption("pubsub", "Output to a Google Cloud PubSub endpoint", f)

	flags := opt.Flags()
	flags.Var(&f.topic, "topic",
		"The Google Cloud PubSub topic name (projects/<project>/topics/<topic>).")
	flags.BoolVar(&f.noCompress, "nocompress", false,
		"Disable compression in published Pub/Sub messages.")

	// TODO(dnj): Default to false when mandatory debugging is finished.
	flags.BoolVar(&f.track, "track", true,
		"Track each sent message. This adds CPU/memory overhead.")

	return opt
}

func (f *pubsubOutputFactory) configOutput(a *application) (output.Output, error) {
	if err := f.topic.Validate(); err != nil {
		return nil, fmt.Errorf("pubsub: invalid topic name: %s", err)
	}

	// Instantiate our Pub/Sub instance. We will use the non-cancelling context,
	// as we want Pub/Sub system to drain without interruption if the application
	// is otherwise interrupted.
	ctx := log.SetFields(a.ncCtx, log.Fields{
		"topic": f.topic,
	})
	ts, err := a.tokenSource(ctx)
	if err != nil {
		return nil, fmt.Errorf("pubsub: failed to initialize Pub/Sub token source: %s", err)
	}

	// Split topic into Pub/Sub project and name.
	project, name := f.topic.Split()

	psClient, err := pubsub.NewClient(ctx, project, cloud.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("pubsub: failed to get Pub/Sub client: %s", err)
	}
	psTopic := psClient.Topic(name)

	// Assert that our Topic exists.
	exists, err := retryTopicExists(ctx, psTopic)
	if err != nil {
		log.WithError(err).Errorf(ctx, "Failed to check for topic.")
		return nil, err
	}
	if !exists {
		log.Fields{
			"topic": f.topic,
		}.Errorf(ctx, "Pub/Sub Topic does not exist.")
		return nil, fmt.Errorf("pubsub: topic %q does not exist", f.topic)
	}

	return out.New(ctx, out.Config{
		Topic:    psTopic,
		Compress: !f.noCompress,
		Track:    f.track,
	}), nil
}

func retryTopicExists(ctx context.Context, t *pubsub.TopicHandle) (bool, error) {
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

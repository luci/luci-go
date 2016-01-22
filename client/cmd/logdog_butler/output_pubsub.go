// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"time"

	"github.com/luci/luci-go/client/internal/logdog/butler/output"
	"github.com/luci/luci-go/client/internal/logdog/butler/output/pubsub"
	"github.com/luci/luci-go/common/flag/multiflag"
	"github.com/luci/luci-go/common/gcloud/gcps"
	log "github.com/luci/luci-go/common/logging"
)

func init() {
	registerOutputFactory(new(pubsubOutputFactory))
}

// pubsubOutputFactory for Google Cloud PubSub.
type pubsubOutputFactory struct {
	topic      gcps.Topic
	project    string
	noCompress bool
}

var _ outputFactory = (*pubsubOutputFactory)(nil)

func (f *pubsubOutputFactory) option() multiflag.Option {
	opt := newOutputOption("pubsub", "Output to a Google Cloud PubSub endpoint", f)

	flags := opt.Flags()
	flags.StringVar(&f.project, "project", "",
		"The Google Cloud project that the Pub/Sub belongs to.")
	flags.Var(&f.topic, "topic",
		"The base Google Cloud PubSub topic name.")
	flags.BoolVar(&f.noCompress, "nocompress", false,
		"Disable compression in published Pub/Sub messages.")

	return opt
}

func (f *pubsubOutputFactory) configOutput(a *application) (output.Output, error) {
	if f.project == "" {
		return nil, fmt.Errorf("pubsub: must supply a project name (-project)")
	}
	if err := f.topic.Validate(); err != nil {
		return nil, fmt.Errorf("pubsub: invalid topic name: %s", err)
	}

	// Instantiate our Pub/Sub instance. We will use the non-cancelling context,
	// as we want Pub/Sub system to drain without interruption if the application
	// is otherwise interrupted.
	ctx := log.SetFields(a.ncCtx, log.Fields{
		"topic":   f.topic,
		"project": f.project,
	})
	client, err := a.authenticatedClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("pubsub: failed to initialize Pub/Sub context: %s", err)
	}
	ps := &gcps.Retry{
		Connection: gcps.NewConnection(client, f.project),
		Callback: func(err error, d time.Duration) {
			log.Fields{
				log.ErrorKey: err,
				"delay":      d,
			}.Warningf(ctx, "Transient error during Pub/Sub operation; retrying...")
		},
	}

	// Assert that our Topic exists.
	exists, err := ps.TopicExists(ctx, f.topic)
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

	return pubsub.New(ctx, pubsub.Config{
		Publisher: ps,
		Topic:     f.topic,
		Compress:  !f.noCompress,
	}), nil
}

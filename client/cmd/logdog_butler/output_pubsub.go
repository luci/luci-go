// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"time"

	"github.com/luci/luci-go/client/internal/flags/multiflag"
	"github.com/luci/luci-go/client/internal/logdog/butler/output"
	"github.com/luci/luci-go/client/internal/logdog/butler/output/pubsub"
	"github.com/luci/luci-go/common/gcloud/gcps"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
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
	opt := newOutputOption("gcps", "Output to a Google Cloud PubSub endpoint", f)

	flags := opt.Flags()
	flags.StringVar(&f.project, "project", "",
		"The Google Cloud project that the Pub/Sub belongs to.")
	flags.Var(&f.topic, "topic",
		"The base Google Cloud PubSub topic name.")
	flags.BoolVar(&f.noCompress, "nocompress", false,
		"Disable compression in published Pub/Sub messages.")

	return opt
}

func (f *pubsubOutputFactory) configOutput(a *butlerApplication) (output.Output, error) {
	if f.project == "" {
		return nil, fmt.Errorf("gcps: must supply a project name (-project)")
	}
	if err := f.topic.Validate(); err != nil {
		return nil, fmt.Errorf("gcps: invalid topic name: %s", err)
	}

	ctx := log.SetFields(a, log.Fields{
		"topic":   f.topic,
		"project": f.project,
	})
	ctx, err := a.authenticatedContext(ctx, f.project)
	if err != nil {
		return nil, fmt.Errorf("gcps: failed to initialize GCPS context: %s", err)
	}
	ps := gcps.New(ctx)

	// Assert that our Topic exists.
	if err := f.assertTopicExists(ctx, ps); err != nil {
		return nil, err
	}

	return pubsub.New(ctx, pubsub.Config{
		PubSub:   ps,
		Topic:    f.topic,
		Compress: !f.noCompress,
	}), nil
}

func (f *pubsubOutputFactory) assertTopicExists(ctx context.Context, ps gcps.PubSub) error {
	log.Infof(ctx, "Checking that Pub/Sub topic exists.")

	exists := false
	err := retry.Retry(ctx, retry.TransientOnly(retry.Default()), func() error {
		e, err := ps.TopicExists(f.topic)
		if err != nil {
			return err
		}
		exists = e
		return nil
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
		}.Warningf(ctx, "Transient error during topic check; retrying.")
	})
	if err != nil {
		return fmt.Errorf("gcps: failed to check for topic: %s", err)
	}
	if !exists {
		return fmt.Errorf("gcps: topic [%s] does not exist", f.topic)
	}
	return nil
}

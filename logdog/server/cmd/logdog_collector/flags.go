// Copyright 2021 The LUCI Authors.
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

package main

import (
	"flag"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/pubsub"
)

// CommandLineFlags contains collector service configuration.
//
// It is exposed via CLI flags.
type CommandLineFlags struct {
	// MaxConcurrentMessages is the maximum number of concurrent transport
	// messages to process.
	//
	// If <= 0, a default will be chosen based on the transport.
	MaxConcurrentMessages int

	// StateCacheSize is the maximum number of log stream states to cache locally.
	//
	// If <= 0, a default will be used.
	StateCacheSize int

	// StateCacheExpiration is the maximum amount of time that cached stream state
	// is valid.
	//
	// If <= 0, a default will be used.
	StateCacheExpiration time.Duration

	// MaxMessageWorkers is the maximum number of concurrent workers to process
	// each ingested message.
	//
	// If <= 0, collector.DefaultMaxMessageWorkers will be used.
	MaxMessageWorkers int

	// PubSubProject is the Cloud Project name that hosts the PubSub subscription
	// that receives incoming logs.
	//
	// Required.
	PubSubProject string

	// PubSubSubscription is the name of the PubSub subscription that receives
	// incoming logs.
	//
	// Required.
	PubSubSubscription string
}

// Register registers flags in the flag set.
func (f *CommandLineFlags) Register(fs *flag.FlagSet) {
	fs.IntVar(&f.MaxConcurrentMessages, "max-concurrent-messages", f.MaxConcurrentMessages,
		"Maximum number of concurrent transport messages to process.")
	fs.IntVar(&f.StateCacheSize, "state-cache-size", f.StateCacheSize,
		"maximum number of log stream states to cache locally.")
	fs.DurationVar(&f.StateCacheExpiration, "state-cache-expiration", f.StateCacheExpiration,
		"Maximum amount of time that cached stream state is valid.")
	fs.IntVar(&f.MaxMessageWorkers, "max-message-workers", f.MaxMessageWorkers,
		"Maximum number of concurrent workers to process each ingested message.")
	fs.StringVar(&f.PubSubProject, "pubsub-project", f.PubSubProject,
		"Cloud Project that hosts the PubSub subscription.")
	fs.StringVar(&f.PubSubSubscription, "pubsub-subscription", f.PubSubSubscription,
		"PubSub subscription within the project.")
}

// Validate returns an error if some parsed flags have invalid values.
func (f *CommandLineFlags) Validate() error {
	if f.PubSubProject == "" {
		return errors.New("-pubsub-project is required")
	}
	if f.PubSubSubscription == "" {
		return errors.New("-pubsub-subscription is required")
	}
	sub := pubsub.NewSubscription(f.PubSubProject, f.PubSubSubscription)
	if err := sub.Validate(); err != nil {
		return errors.Fmt("invalid Pub/Sub subscription %q: %w", sub, err)
	}
	return nil
}

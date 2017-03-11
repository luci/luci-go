// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package pubsub

import (
	"flag"

	"cloud.google.com/go/pubsub"
)

// Topic is a fully-qualified Pub/Sub project/topic name.
type Topic string

var _ flag.Value = (*Topic)(nil)

// NewTopic generates a new Topic for a given project and topic name.
func NewTopic(project, name string) Topic {
	return Topic(newResource(project, "topics", name))
}

func (t *Topic) String() string {
	return string(*t)
}

// Set implements flag.Value.
func (t *Topic) Set(value string) error {
	v := Topic(value)
	if err := v.Validate(); err != nil {
		return err
	}
	*t = v
	return nil
}

// Validate returns an error if the topic name is invalid.
func (t Topic) Validate() error {
	return validateResource(string(t), "topics")
}

// Split returns the Topic's project component. If no project is
// defined (malformed), an empty string will be returned.
func (t Topic) Split() (p, n string) {
	p, n, _ = t.SplitErr()
	return
}

// SplitErr returns the Topic's project and name components.
func (t Topic) SplitErr() (p, n string, err error) {
	p, n, err = resourceProjectName(string(t))
	return
}

// DisableTopicBundling configures a new pubsub.Topic to not bundle its data.
//
// t must NOT have had any Publish calls made yet, else these settings will not
// have any effect.
//
// By default, a Pub/Sub Topic bundles data, delaying its actual dispatch in
// favor of batching calls. Sometimes this is not desired. This function
// configures an existing Topic not to bundle.
func DisableTopicBundling(t *pubsub.Topic) {
	t.PublishSettings.DelayThreshold = 0
	t.PublishSettings.NumGoroutines = 1
	t.PublishSettings.CountThreshold = 1
}

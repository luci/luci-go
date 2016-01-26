// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pubsub

import (
	"flag"
)

// Topic is a fully-qualified Pub/Sub project/topic name.
type Topic string

var _ flag.Value = (*Topic)(nil)

// NewTopic generates a new Subscritpion for a given project and topic name.
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

// Project returns the Topic's project component. If no project is defined
// (malformed), an empty string will be returned.
func (t Topic) Project() (v string) {
	v, _ = t.ProjectErr()
	return
}

// ProjectErr returns the Subscription's project component.
func (t Topic) ProjectErr() (v string, err error) {
	v, err = resourceProject(string(t))
	return
}

// Name returns the Topic's name component. If no name is defined (malformed),
// an empty string will be returned.
func (t Topic) Name() (v string) {
	v, _ = resourceName(string(t))
	return
}

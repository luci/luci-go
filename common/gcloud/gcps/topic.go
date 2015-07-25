// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gcps

import (
	"flag"
)

// Topic is a Pub/Sub topic name.
type Topic string

var _ flag.Value = (*Topic)(nil)

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

// Path returns the topic's endpoint path.
func (t Topic) Path(project string) string {
	return resourcePath(project, "topics", string(t))
}

// Validate returns an error if the topic name is invalid.
func (t Topic) Validate() error {
	return validateResource(string(t))
}

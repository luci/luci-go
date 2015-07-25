// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gcps

import (
	"flag"
)

// Subscription is a Pub/Sub subscription name.
type Subscription string

var _ flag.Value = (*Subscription)(nil)

func (s *Subscription) String() string {
	return string(*s)
}

// Set implements flag.Value.
func (s *Subscription) Set(value string) error {
	v := Subscription(value)
	if err := v.Validate(); err != nil {
		return err
	}
	*s = v
	return nil
}

// Path returns the subscription's endpoint path.
func (s Subscription) Path(project string) string {
	return resourcePath(project, "subscriptions", string(s))
}

// Validate returns an error if the subscription name is invalid.
func (s Subscription) Validate() error {
	return validateResource(string(s))
}

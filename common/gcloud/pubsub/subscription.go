// Copyright 2015 The LUCI Authors.
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

package pubsub

import (
	"flag"
)

// Subscription is a Pub/Sub subscription name.
type Subscription string

var _ flag.Value = (*Subscription)(nil)

// NewSubscription generates a new Subscription for a given project and
// subscription name.
func NewSubscription(project, name string) Subscription {
	return Subscription(newResource(project, "subscriptions", name))
}

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

// Validate returns an error if the subscription name is invalid.
func (s Subscription) Validate() error {
	return validateResource(string(s), "subscriptions")
}

// Split returns the Subscription's project component. If no project is
// defined (malformed), an empty string will be returned.
func (s Subscription) Split() (p, n string) {
	p, n, _ = s.SplitErr()
	return
}

// SplitErr returns the Subscription's project and name components.
func (s Subscription) SplitErr() (p, n string, err error) {
	p, n, err = resourceProjectName(string(s))
	return
}

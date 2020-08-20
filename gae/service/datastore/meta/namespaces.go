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

package meta

import (
	"fmt"
	"strings"

	ds "go.chromium.org/gae/service/datastore"

	"golang.org/x/net/context"
)

// NamespacesCallback is the callback type used with Namespaces. The callback
// will be invoked with each identified namespace.
//
// If the callback returns an error, iteration will stop. If the error is
// datastore.Stop, Namespaces will stop iterating and return nil. Otherwise,
// the error will be forwarded.
type NamespacesCallback func(string) error

// Namespaces returns a list of all of the namespaces in the datastore.
//
// This is done by issuing a datastore query for kind "__namespace__". The
// resulting keys will have IDs for the namespaces, namely:
//	- The empty namespace will have integer ID 1. This will be forwarded to the
//	  callback as "".
//	- Other namespaces will have non-zero string IDs.
func Namespaces(c context.Context, cb NamespacesCallback) error {
	q := ds.NewQuery("__namespace__").KeysOnly(true)

	// Query our datastore for the full set of namespaces.
	return ds.Run(c, q, func(k *ds.Key) error {
		switch {
		case k.IntID() == 1:
			return cb("")
		case k.IntID() != 0:
			return fmt.Errorf("unexpected namepsace integer key (%d)", k.IntID())
		default:
			return cb(k.StringID())
		}
	})
}

// NamespacesWithPrefix runs Namespaces, returning only namespaces beginning
// with the supplied prefix string.
func NamespacesWithPrefix(c context.Context, p string, cb NamespacesCallback) error {
	// TODO: https://go.chromium.org/gae/issues/49 : When inequality filters are
	// supported, implement this using a "Gte" filter.
	any := false
	return Namespaces(c, func(ns string) error {
		if !strings.HasPrefix(ns, p) {
			if any {
				return ds.Stop
			}
			return nil
		}

		any = true
		return cb(ns)
	})
}

// NamespacesCollector exposes a NamespacesCallback function that aggregates
// resulting namespaces into the collector slice.
type NamespacesCollector []string

// Callback is a NamespacesCallback which adds each namespace to the collector.
func (c *NamespacesCollector) Callback(v string) error {
	*c = append(*c, v)
	return nil
}

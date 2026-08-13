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
	"context"
	"fmt"
	"iter"
	"strings"

	ds "go.chromium.org/luci/gae/service/datastore"
)

// Namespaces returns a list of all of the namespaces in the datastore.
//
// This is done by issuing a datastore query for kind "__namespace__". The
// resulting keys will have IDs for the namespaces, namely:
//   - The empty namespace will have integer ID 1. This will be yielded as "".
//   - Other namespaces will have non-zero string IDs.
func Namespaces(c context.Context) iter.Seq2[string, error] {
	q := ds.NewQuery("__namespace__").KeysOnly(true)

	return func(yield func(string, error) bool) {
		// Query our datastore for the full set of namespaces.
		for k, err := range ds.RunQuery[*ds.Key](c, q).Results {
			if err != nil {
				yield("", err)
				return
			}

			var stop bool
			switch {
			case k.IntID() == 1:
				stop = !yield("", nil)
			case k.IntID() != 0:
				yield("", fmt.Errorf("unexpected namespace integer key (%d)", k.IntID()))
				return
			default:
				stop = !yield(k.StringID(), nil)
			}
			if stop {
				return
			}
		}
	}
}

// NamespacesWithPrefix runs Namespaces, returning only namespaces beginning
// with the supplied prefix string.
func NamespacesWithPrefix(c context.Context, p string) iter.Seq2[string, error] {
	// TODO: https://github.com/luci/gae/issues/49 : When inequality filters are
	// supported, implement this using a "Gte" filter.
	foundOne := false
	return func(yield func(string, error) bool) {
		for ns, err := range Namespaces(c) {
			if err != nil {
				yield("", err)
				return
			}

			if !strings.HasPrefix(ns, p) {
				if foundOne {
					return
				}
				continue
			}
			foundOne = true
			if !yield(ns, nil) {
				return
			}
		}
	}
}

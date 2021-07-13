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

package gerrit

import (
	"context"
	"math/rand"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"google.golang.org/grpc"
)

// MirrorIteratorFactory makes MirrorIterator to sequentially route request to
// Gerrit virtual host at first and then to specific mirrors.
//
// Empty value and nil value are valid and mean that there are no mirrors.
type MirrorIteratorFactory struct {
	MirrorHostPrefixes []string
}

// Make makes a new MirrorIterator.
func (f *MirrorIteratorFactory) Make(ctx context.Context) *MirrorIterator {
	// First prefix "" means use host as is.
	it := &MirrorIterator{""}
	if f == nil {
		return it
	}
	// Copied from the factory.
	*it = append(*it, f.MirrorHostPrefixes...)
	// Shuffle factory-provided prefixes.
	nonEmpty := (*it)[1:]
	_ = mathrand.WithGoRand(ctx, func(r *rand.Rand) error {
		rand.Shuffle(len(nonEmpty), func(i, j int) { nonEmpty[i], nonEmpty[j] = nonEmpty[j], nonEmpty[i] })
		return nil
	})
	return it
}

// MirrorIterator starts with the Gerrit host as is and then iterates over
// mirrors.
//
// Must be constructed using its MirrorIteratorFactory.
//
// Fail-safe: if all mirrors have been used, uses the host as is. However,
// users should use Empty() to detect this and if necessary should construct a
// new MirrorIterator.
//
// Not goroutine-safe.
type MirrorIterator []string

// Next returns a grpc.CallOption.
func (it *MirrorIterator) Next() grpc.CallOption {
	return gerrit.UseGerritMirror(it.next())
}

// next is an easily testable version of Next().
func (it *MirrorIterator) next() func(host string) string {
	prefix := "" // fail-safe to host as is when empty.
	if len(*it) > 0 {
		prefix = (*it)[0]
		*it = (*it)[1:]
	}
	return func(host string) string { return prefix + host }
}

// Empty returns true if MirrorIterator has already iterated over all the mirrors.
func (it *MirrorIterator) Empty() bool {
	return len(*it) == 0
}

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
	"sync"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"google.golang.org/grpc"
)

// MirrorIteratorFactory makes MirrorIterator to sequentially route request to
// Gerrit virtual host at first and then to specific mirrors.
//
// Empty value and nil value are valid and means that there are no mirrors.
type MirrorIteratorFactory struct {
	MirrorHostPrefixes []string
}

// Make makes a new MirrorIterator.
func (f *MirrorIteratorFactory) Make(ctx context.Context) *MirrorIterator {
	it := &MirrorIterator{}
	if f == nil {
		it.prefixes = []string{""}
	} else {
		// First prefix "" means use host as is, the rest are copied from the factory.
		prefixes := append([]string{""}, f.MirrorHostPrefixes...)
		// Shuffle non-"" prefixes.
		nonEmpty := prefixes[1:]
		_ = mathrand.WithGoRand(ctx, func(r *rand.Rand) error {
			rand.Shuffle(len(nonEmpty), func(i, j int) { nonEmpty[i], nonEmpty[j] = nonEmpty[j], nonEmpty[i] })
			return nil
		})
		it.prefixes = prefixes
	}
	it.CallOption = gerrit.UseGerritMirror(it.Next)
	return it
}

// MirrorIterator starts with the Gerrit host as is and then iterates over
// mirrors.
//
// Must be constructed using its MirrorIteratorFactory.
//
// Fail-safe: if all mirrors have been used, uses the host as is.  However,
// users should use Empty() to detect this and if necessary should construct a
// new MirrorIterator.
type MirrorIterator struct {
	grpc.CallOption
	prefixes []string
	m        sync.Mutex
}

// Next returns the next host to use instead of the given one.
func (it *MirrorIterator) Next(host string) string {
	it.m.Lock()
	if len(it.prefixes) > 0 {
		host = it.prefixes[0] + host
		it.prefixes = it.prefixes[1:]
	}
	// else, default to host as is.
	it.m.Unlock()
	return host
}

// Empty returns true if MirrorIterator has already iterated over all the mirrors.
func (it *MirrorIterator) Empty() bool {
	it.m.Lock()
	empty := len(it.prefixes) == 0
	it.m.Unlock()
	return empty
}

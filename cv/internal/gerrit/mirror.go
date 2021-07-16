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

// MirrorIterator starts with the Gerrit host as is and then iterates over
// mirrors.
//
// Fail-safe: if all mirrors have been used, uses the host as is. However,
// users should use Empty() to detect this and if necessary should construct a
// new MirrorIterator.
//
// Not goroutine-safe.
type MirrorIterator []string

func newMirrorIterator(ctx context.Context, mirrorHostPrefixes ...string) *MirrorIterator {
	// First prefix "" means use host as is.
	it := &MirrorIterator{""}
	*it = append(*it, mirrorHostPrefixes...) // copy
	// Shuffle the provided prefixes.
	provided := (*it)[1:]
	_ = mathrand.WithGoRand(ctx, func(r *rand.Rand) error {
		rand.Shuffle(len(provided), func(i, j int) { provided[i], provided[j] = provided[j], provided[i] })
		return nil
	})
	return it
}

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

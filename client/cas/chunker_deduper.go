// Copyright 2020 The LUCI Authors.
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

package cas

import (
	"sync"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/chunker"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
)

// ChunkerDeduper deduplicates the chunkers uploaded to CAS. Each time a CAS
// merkle tree is constructed, we should use this to dedupe the chunkers on the
// client side before uploading to the server. This is thread safe.
type ChunkerDeduper struct {
	mu       sync.Mutex
	uploaded map[digest.Digest]struct{}
}

// NewChunkerDeduper creates a new ChunkerDeduper object.
func NewChunkerDeduper() *ChunkerDeduper {
	return &ChunkerDeduper{
		uploaded: make(map[digest.Digest]struct{}),
	}
}

// Deduplicate removes the chunkers that have already been requested to be
// uploaded to CAS.
func (cd *ChunkerDeduper) Deduplicate(chunkers []*chunker.Chunker) []*chunker.Chunker {
	var deduped []*chunker.Chunker
	cd.mu.Lock()
	defer cd.mu.Unlock()
	for _, c := range chunkers {
		dg := c.Digest()
		if _, ok := cd.uploaded[dg]; ok {
			continue
		}
		deduped = append(deduped, c)
		cd.uploaded[dg] = struct{}{}
	}
	return deduped
}

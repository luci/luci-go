// Copyright 2026 The LUCI Authors.
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

package prefix

import (
	"iter"
	"strings"
)

// Walk traverses `prefix` and its descendants across `tracks` in depth-first
// order, yielding each normalized prefix path (`""` for root, `"a/b"` without
// trailing slash for non-root) where at least one track establishes (at
// `prefix`) or updates its value.
//
// Inside the loop body, callers inspect track state via [Track.Value] and may
// dynamically redirect or short-circuit subtree traversal via [Track.SetTree]
// and [Track.Prune].
func Walk(prefix string, tracks ...GenericTrack) iter.Seq[string] {
	return func(yield func(string) bool) {
		if len(tracks) == 0 {
			return
		}

		all := trackSlice(tracks)

		// prefixes mirrors each track's ancestor stack, recording the normalized
		// prefix at each active ancestor depth.
		prefixes := []string{normPath(prefix)}
		if all.push(prefixes[0], true) && !yield(strings.TrimSuffix(prefixes[0], "/")) {
			return
		}

		for len(prefixes) > 0 {
			next, ok := all.nextPrefix(prefixes[len(prefixes)-1])
			if !ok {
				prefixes = prefixes[:len(prefixes)-1]
				all.pop()
				continue
			}

			prefixes = append(prefixes, next)
			if all.push(next, false) && !yield(strings.TrimSuffix(next, "/")) {
				return
			}
		}
	}
}

// trackSlice is just an aggregated version of [GenericTrack].
type trackSlice []GenericTrack

// nextPrefix computes the next normalized prefix below `curPrefix` to step to,
// selecting the lexicographically smallest outgoing child branch across all
// tracks and fast-forwarding across any mid-edge segments shared by all tracks
// on that branch.
func (s trackSlice) nextPrefix(curPrefix string) (next string, ok bool) {
	for _, track := range s {
		p, has := track.peek()
		if !has {
			continue
		}
		if !ok {
			next, ok = p, true
			continue
		}
		if common := commonPrefixLen(next, p); common > len(curPrefix) {
			next = next[:common]
		} else if p < next {
			next = p
		}
	}
	return next, ok
}

func (s trackSlice) push(nextPrefix string, init bool) (anyChanged bool) {
	for _, track := range s {
		if track.push(nextPrefix, init) {
			anyChanged = true
		}
	}
	return
}

func (s trackSlice) pop() {
	for _, track := range s {
		track.pop()
	}
}

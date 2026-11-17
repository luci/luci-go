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
	"cmp"
	"strings"
)

// normPath normalizes a '/' separated prefix path to "" for root and a
// trailing-slash path ("a/b/c/") for non-root.
func normPath(p string) string {
	// Try to avoid new allocations - most of the time p is already normalized.
	//
	// Cut away all prefix slashes, and any double slashes on the suffix. Both
	// of these operations should be fast no-ops roughly 100% of the time.
	p = strings.TrimLeft(p, "/")
	for {
		reduced, ok := strings.CutSuffix(p, "//")
		if !ok {
			break
		}
		p = reduced
	}
	// If it's empty or doesn't have double slashes, return it.
	if p == "" {
		return ""
	}
	if !strings.Contains(p, "//") {
		if p[len(p)-1] == '/' {
			return p
		}
		return p + "/"
	}

	// Otherwise, rebuild the new normalized path.
	var b strings.Builder
	b.Grow(len(p) + 1)
	for {
		seg, rest, ok := nextSegment(p)
		if !ok {
			return b.String()
		}
		b.WriteString(seg)
		b.WriteByte('/')
		p = rest
	}
}

// nextSegment yields the next non-empty '/' separated segment (without '/')
// from possibly-unnormalized `path`.
func nextSegment(path string) (seg, next string, ok bool) {
	for path != "" {
		seg, path, _ = strings.Cut(path, "/")
		if seg != "" {
			return seg, path, true
		}
	}
	return "", "", false
}

// cutSlashSegment splits a non-empty slash-terminated path ("b/c/") into its
// first slash-terminated segment ("b/") and the remainder ("c/").
func cutSlashSegment(clean string) (segWithSlash, rest string) {
	idx := strings.IndexByte(clean, '/')
	return clean[:idx+1], clean[idx+1:]
}

// compareFirstSegment compares the first slash-terminated segment of `edge`
// against `targetSeg + "/"` (where `targetSeg` contains no '/').
func compareFirstSegment(edge, targetSeg string) int {
	for i := range min(len(edge), len(targetSeg)) {
		if diff := cmp.Compare(edge[i], targetSeg[i]); diff != 0 {
			return diff
		}
	}
	return cmp.Compare(edge[len(targetSeg)], '/')
}

// commonPrefixLen returns the byte length (including trailing '/') of the
// longest common segment prefix shared by two slash-terminated paths `a` and
// `b` (0 if they share no segments).
func commonPrefixLen(a, b string) int {
	lastSlash := 0
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return lastSlash
		}
		if a[i] == '/' {
			lastSlash = i + 1
		}
	}
	return lastSlash
}

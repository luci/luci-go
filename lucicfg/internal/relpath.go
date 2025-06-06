// Copyright 2025 The LUCI Authors.
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

package internal

import (
	"path"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// RelRepoPath calculates a relative path from `base` to `target`.
//
// Both paths are assumed to be relative paths within the same repository: they
// are slash-separated and should not start with "../" after being cleaned (they
// may be "." though, indicating the repository root).
func RelRepoPath(base, target string) (string, error) {
	baseChunks, err := splitPath(base)
	if err != nil {
		return "", err
	}
	targetChunks, err := splitPath(target)
	if err != nil {
		return "", err
	}

	// Remove common prefix, it doesn't affect relative path.
	for len(baseChunks) > 0 && len(targetChunks) > 0 {
		if baseChunks[0] != targetChunks[0] {
			break
		}
		baseChunks = baseChunks[1:]
		targetChunks = targetChunks[1:]
	}

	if len(baseChunks) == 0 && len(targetChunks) == 0 {
		return ".", nil // the exact same path
	}

	// Step out of "base" to the common ancestor with "target", then step down to
	// the target.
	out := make([]string, 0, len(baseChunks)+len(targetChunks))
	for range baseChunks {
		out = append(out, "..")
	}
	out = append(out, targetChunks...)
	return strings.Join(out, "/"), nil
}

func splitPath(p string) ([]string, error) {
	switch chunks := strings.Split(path.Clean(p), "/"); {
	case chunks[0] == "..":
		return nil, errors.Fmt("bad repo path %q, goes outside of the repo root", p)
	case chunks[0] == ".":
		return nil, nil // empty path indicating the root of the repo
	default:
		return chunks, nil
	}
}

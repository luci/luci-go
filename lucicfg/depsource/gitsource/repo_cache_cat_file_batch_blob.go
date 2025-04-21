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

package gitsource

import (
	"bytes"
	"context"
	"fmt"
)

func (b *batchProc) catFileBlob(ctx context.Context, commit, path string) ([]byte, error) {
	kind, data, err := b.catFile(ctx, fmt.Sprintf("%s:%s", commit, path))
	if err != nil {
		return nil, err
	}
	if kind != BlobKind {
		return nil, fmt.Errorf("not a blob: %s", kind)
	}
	return data, nil
}

// ReadSingleFile will read a single blob `commit:path`.
//
// Note that `path` is relative to the commit root.
//
// This IS a cached operation, however it may be as slow as fetching many files
// via [RepoCache.Fetcher]'s prefetch function.
func (r *RepoCache) ReadSingleFile(ctx context.Context, commit, path string) ([]byte, error) {
	ctx = r.prepDebugContext(ctx)

	// notably, this does NOT have --no-lazy-fetch.
	output, err := r.gitCombinedOutput(ctx, "cat-file", "blob", fmt.Sprintf("%s:%s", commit, path))
	if err == nil {
		return output, nil
	}
	if bytes.Contains(output, []byte("not our ref")) {
		err = ErrMissingCommit
	} else if bytes.Contains(output, []byte("does not exist")) {
		err = ErrMissingObject
	}
	return nil, err
}

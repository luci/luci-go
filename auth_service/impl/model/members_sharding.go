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

package model

import (
	"context"
	"strings"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/auth_service/internal/zstd"
)

type MemberShard interface {
	GetBlob() []byte
}

// shardMembers splits the given members into shards of size maxSize and
// returns them. If sharding is not necessary (i.e. members is <= maxSize), this
// returns nil.
func shardMembers(ctx context.Context, members []string, maxSize int) [][]byte {
	if len(members) == 0 {
		return nil
	}

	total := 0
	for _, m := range members {
		// Strings in Datastore are number of utf-8 bytes + 1 (see
		// https://docs.cloud.google.com/datastore/docs/concepts/storage-size#string_size).
		total += len(m) + 1
	}
	if total <= maxSize {
		// No sharding necessary.
		return nil
	}

	// Compress the blob to reduce the number of shards needed.
	rawBlob := []byte(strings.Join(members, ","))
	blob := make([]byte, 0, len(rawBlob)/2) // Hope for at least 2x compression.
	blob = zstd.Compress(rawBlob, blob)

	count := (len(blob) + maxSize - 1) / maxSize
	shards := make([][]byte, count)
	for i := range count {
		idxStart := i * maxSize
		idxEnd := idxStart + maxSize
		if idxEnd > len(blob) {
			idxEnd = len(blob)
		}
		shards[i] = blob[idxStart:idxEnd]
	}

	return shards
}

// unshardMembers reverses the sharding process to get the members value from
// the given shards.
func unshardMembers[T MemberShard](ctx context.Context, shards []T) ([]string, error) {
	if len(shards) == 0 {
		return nil, nil
	}

	// Reconstruct the compressed blob.
	var blob []byte
	for _, shard := range shards {
		blob = append(blob, shard.GetBlob()...)
	}

	// Decompress the blob.
	uncompressedBlob, err := zstd.Decompress(blob, nil)
	if err != nil {
		return nil, errors.Fmt("error decompressing members blob: %w", err)
	}

	return strings.Split(string(uncompressedBlob), ","), nil
}

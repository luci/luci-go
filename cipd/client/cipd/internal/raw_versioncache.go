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

package internal

import (
	"bytes"
	"context"
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
	"go.chromium.org/luci/cipd/common/cipderr"
)

func mkVersionCachePaths(cacheDir fs.FileSystem) (curPath, legacyPath string, err error) {
	curPath, err = cacheDir.RootRelToAbs(versionCacheName)
	if err != nil {
		return "", "", cipderr.BadArgument.Apply(errors.Fmt("bad version cache path: %w", err))
	}
	legacyPath, err = cacheDir.RootRelToAbs(legacyVersionCacheName)
	if err != nil {
		return "", "", cipderr.BadArgument.Apply(errors.Fmt("bad version cache path: %w", err))
	}
	return curPath, legacyPath, nil
}

// ReadVersionCache loads and parses a version cache file.
//
// `cacheDir` should be a cipd cache directory.
//
// Returns nil if the file is missing or corrupted.
// Returns errors if the file can't be read.
func ReadVersionCache(ctx context.Context, cacheDir fs.FileSystem) (ret *messages.VersionCache, err error) {
	curPath, legacyPath, err := mkVersionCachePaths(cacheDir)
	if err != nil {
		return nil, err
	}

	var blob []byte
	// Load from the new path or the legacy path.
	for _, pickedPath := range []string{curPath, legacyPath} {
		blob, err = os.ReadFile(pickedPath)
		switch {
		case os.IsNotExist(err):
			continue
		case err != nil:
			return nil, cipderr.IO.Apply(errors.Fmt("reading version cache: %w", err))
		}
		break
	}
	if os.IsNotExist(err) {
		return nil, nil
	}

	cache := &messages.VersionCache{}
	if err := UnmarshalWithSHA256(blob, cache); err != nil {
		// Just ignore the corrupted cache file.
		logging.Warningf(ctx, "Can't deserialize version cache: %s", err)
		return nil, nil
	}

	return cache, nil
}

// LegacyVersionCache indicates that this use of VersionCache should use the
// legacy name when writing the cache.
//
// All TagCache instances will attempt to read the old and new names.
type LegacyVersionCache bool

const (
	// When writing a tagcache, use the current name and remove the legacy one if
	// it exists.
	UseModernVCName LegacyVersionCache = false
	// When writing a tagcache, use the legacy name.
	UseLegacyVCName LegacyVersionCache = true
)

// WriteVersionCache serializes the version cache and writes it to the file system.
func WriteVersionCache(ctx context.Context, cacheDir fs.FileSystem, msg *messages.VersionCache, useName LegacyVersionCache) error {
	var name = versionCacheName
	if useName == UseLegacyVCName {
		name = legacyVersionCacheName
	}
	path, err := cacheDir.RootRelToAbs(name)
	if err != nil {
		return cipderr.BadArgument.Apply(errors.Fmt("bad version cache path: %w", err))
	}

	blob, err := MarshalWithSHA256(msg)
	if err != nil {
		return cipderr.BadArgument.Apply(errors.Fmt("serializing version cache: %w", err))
	}

	if err := fs.EnsureFile(ctx, cacheDir, path, bytes.NewReader(blob)); err != nil {
		return cipderr.IO.Apply(errors.Fmt("writing version cache: %w", err))
	}

	return nil
}

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

package cipd

import (
	"context"
	"path/filepath"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// CacheRegister registers a package instance, its tags, and its refs directly
// into a local CIPD cache directory without uploading to the CIPD backend.
func CacheRegister(ctx context.Context, serviceURL, cacheDir string, src pkg.Source, pin common.Pin, tags, refs []string) error {
	if !filepath.IsAbs(cacheDir) {
		return cipderr.BadArgument.Apply(errors.Fmt("cacheDir must be an absolute path, got %q", cacheDir))
	}

	instCache := internal.InstanceCache{
		FS: fs.NewFileSystem(filepath.Join(cacheDir, instancesSubdir), ""),
	}
	if err := instCache.Put(ctx, pin, src); err != nil {
		return err
	}

	// Add tags and refs to cache
	if len(tags) > 0 || len(refs) > 0 {
		vc := &internal.VersionCache{
			FS:                     fs.NewFileSystem(cacheDir, ""),
			MaxTags:                -1,
			MaxRefs:                -1,
			MaxExtractedObjectRefs: -1,
		}
		for _, tag := range tags {
			if err := vc.AddTag(ctx, serviceURL, pin, tag); err != nil {
				return err
			}
		}
		for _, ref := range refs {
			if err := vc.AddRef(ctx, serviceURL, pin, ref); err != nil {
				return err
			}
		}
		if err := vc.Flush(ctx); err != nil {
			return cipderr.IO.Apply(errors.Fmt("flushing version cache: %w", err))
		}
	}
	return nil
}

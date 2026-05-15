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
	"maps"
	"path/filepath"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/client/cipd/ui"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// CachePrepare will prepare `cacheDir` as a standard CIPD cache
// directory.
//
// It will contain all instances described by `versions`, plus a versioncache
// of any unresolved versions.
//
// If the directory already exists and `trim` is true, it will be garbage
// collected to contain exactly `versions` instances, tags and refs. Otherwise,
// this will merely accumulate new instances into the existing directory.
//
// This is a standalone function to allow `versions` to contain versions from
// multiple different service urls.
func CachePrepare(ctx context.Context, opts ClientOptions, cacheDir string, trim bool, versions PackageVersionSet) error {
	if !filepath.IsAbs(cacheDir) {
		return cipderr.BadArgument.Apply(errors.Fmt("CachePrepare: must be given an absolute path, got: %q", cacheDir))
	}
	if cacheDir == opts.CacheDir {
		return cipderr.BadArgument.Apply(errors.Fmt("CachePrepare: cannot prepare offline cache in same directory as $%s: %q", EnvCacheDir, cacheDir))
	}

	// We can use `cacheDir` as a read-only cache dir for the client when doing
	// prepwork.
	opts.ReadOnlyCacheDir = cacheDir

	versions = maps.Clone(versions)
	clients := &clientPool{opts: opts}
	defer clients.close(ctx)

	clients.beginBatch(ctx)
	defer clients.endBatch(ctx)

	targetVerCache := &internal.VersionCache{
		FS: fs.NewFileSystem(cacheDir, ""),
		// We don't want to have any limits on how much we can save while preparing
		// the cache.
		MaxTags:                -1,
		MaxExtractedObjectRefs: -1,
		MaxRefs:                -1,
	}
	if trim {
		// If we are trimming, ignore all existing values in the cache dir when
		// reading.
		targetVerCache.Tags = internal.DisableRead
		targetVerCache.FileObjectRefs = internal.DisableRead
		targetVerCache.Refs = internal.DisableRead
	}
	pins, err := versions.resolve(ctx, targetVerCache, clients.resolve, nil)
	if err != nil {
		return err
	}
	if err := targetVerCache.Flush(ctx); err != nil {
		return err
	}

	targetInstCache := internal.ManagedInstanceCache{
		Caches: []*internal.InstanceCache{
			{
				FS:      fs.NewFileSystem(filepath.Join(cacheDir, instancesSubdir), ""),
				Fetcher: clients.fetchInstanceTo,
			},
		},
		ParallelDownloads:          max(0, opts.ParallelDownloads),
		ParallelDownloadsFastStart: true,
	}
	if trim {
		targetInstCache.Caches[0].GCMaxAge = internal.GCOlderThanLaunch
		targetInstCache.Caches[0].ExactGC = true
	} else {
		targetInstCache.Caches[0].PassiveWritePolicy = internal.DisableGC
	}
	defer targetInstCache.Close(ctx)

	activities := &ui.ActivityGroup{}

	reqs := make([]*internal.InstanceRequest, 0, len(pins))
	for pin := range pins {
		fetchCtx, done := ui.NewActivity(ctx, activities, "fetch")
		req := &internal.InstanceRequest{
			Context:    fetchCtx,
			Done:       done,
			ServiceURL: pin.ServiceURL,
			Pin: common.Pin{
				PackageName: pin.PackageName,
				InstanceID:  pin.Version,
			},
			OpenAs: internal.VerifiedInstance,
		}
		reqs = append(reqs, req)
	}
	targetInstCache.RequestInstances(ctx, reqs)

	var errs []error

	for targetInstCache.HasPendingFetches() {
		res := targetInstCache.WaitInstance()
		if res.Err != nil {
			if reader.IsCorruptionError(res.Err) && res.State == nil {
				refetchCtx, refetchDone := ui.NewActivity(ctx, activities, "refetch")
				req := &internal.InstanceRequest{
					Context:    refetchCtx,
					Done:       refetchDone,
					ServiceURL: res.ServiceURL,
					Pin:        res.Pin,
					OpenAs:     internal.VerifiedInstance,
					State:      struct{}{}, // Just enough to make it not-nil.
				}
				targetInstCache.RequestInstances(ctx, []*internal.InstanceRequest{req})
				continue
			}

			errs = append(errs, res.Err)
		} else if err := res.Instance.Close(ctx, false); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

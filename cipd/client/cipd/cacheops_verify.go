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
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/client/cipd/ui"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

func validateVersionCache(ctx context.Context, opts ClientOptions, cacheDir string, clean bool) error {
	cdfs := fs.NewFileSystem(cacheDir, "")
	claimedVersions, err := internal.ReadVersionCache(ctx, cdfs)
	if err != nil {
		logging.Warningf(ctx, "Could not read current version cache: %s", err)
		if clean {
			if err := internal.EnsureVersionCacheGone(ctx, cdfs); err != nil {
				logging.Warningf(ctx, "Purging old version cache failed: %s", err)
			}
		}
		return err
	}
	if claimedVersions == nil {
		return nil
	}

	actualVersions := &internal.VersionCache{
		FS: cdfs,

		// Don't load or retain any existing tag or ref entries.
		Tags: internal.DisableRead | internal.DisableMerge,
		Refs: internal.DisableRead | internal.DisableMerge,

		// We don't support FileObjectRefs, pass them through.
		FileObjectRefs: internal.Passthrough,
	}
	if clean {
		// We don't want to have any limits on how much we can save while verifying
		// the cache.
		actualVersions.MaxTags = -1
		actualVersions.MaxExtractedObjectRefs = -1
		actualVersions.MaxRefs = -1

		// If cleaning, we will overwrite any existing entries regardless of error.
		// Our cache starts empty, so even in the case of a total disaster this
		// will just write an empty cache.
		defer func() {
			logging.Infof(ctx, "Writing cleaned version cache...")
			if err := internal.EnsureVersionCacheGone(ctx, cdfs); err != nil {
				logging.Errorf(ctx, "Unable to drop old cache while cleaning: %s", err)
			}
			if err := actualVersions.Flush(ctx); err != nil {
				logging.Warningf(ctx, "Writing cleaned version cache failed: %s", err)
			}
		}()
	}

	if fe := len(claimedVersions.FileEntries); fe > 0 {
		logging.Warningf(ctx, "Version cache has %d file entries; unable to verify FileObjectRefs.", fe)
	}

	vers := make(PackageVersionSet, len(claimedVersions.Entries)+len(claimedVersions.RefEntries))
	for _, tag := range claimedVersions.Entries {
		vers.Add(tag.Service, tag.Package, tag.Tag)
	}
	for _, ref := range claimedVersions.RefEntries {
		vers.Add(ref.Service, ref.Package, ref.InstanceId)
	}

	clients := &clientPool{opts: opts}
	defer clients.close(ctx)

	clients.beginBatch(ctx)
	defer clients.endBatch(ctx)

	// Resolve into an in-memory version cache.
	resolvedVC := &internal.VersionCache{}
	resolvedPins, err := vers.resolve(ctx, resolvedVC, clients.resolve, clients.checkInstance)
	if err != nil {
		logging.Warningf(ctx, "errors while resolving: %s", err)
		// We can proceed to see more errors via logs as we iterate tags and refs
		// below.
	}

	// Now, check all claimed tags.
	//
	// If they are good, add them to actualVersions.
	badVersions := false
	for _, tag := range claimedVersions.Entries {
		actual, err := resolvedVC.ResolveTag(ctx, tag.Service, tag.Package, tag.Tag)
		if err != nil {
			// This tag is syntactically invalid; drop it - this cannot happen
			// outside of weird test scenarios (like where the server is mocked to
			// return a successful ResolveInstance response with a bad instance ID,
			// or this client is so old that it doesn't know about a new instance ID
			// format).
			logging.Warningf(ctx, "Ignoring bad version %s %s@%s: %s", tag.Service, tag.Package, tag.Tag, err)
			continue
		}
		if actual.InstanceID != tag.InstanceId {
			badVersions = true
			logging.Errorf(ctx, "Bad cached tag: %s: %s@%s: cached=%q actual=%q",
				tag.Service, tag.Package, tag.Tag, tag.InstanceId, actual.InstanceID)
		} else {
			actualVersions.AddTag(ctx, tag.Service, common.Pin{PackageName: tag.Package, InstanceID: actual.InstanceID}, tag.Tag)
		}
	}
	// Then, check all claimed ref InstanceIDs.
	//
	// If they are good, add them to actualVersions.
	for _, ref := range claimedVersions.RefEntries {
		key := PackageVersion{Service: ref.Service, PackageName: ref.Package, Version: ref.InstanceId}
		if _, ok := resolvedPins[key]; ok {
			actualVersions.AddRef(ctx, ref.Service, common.Pin{PackageName: ref.Package, InstanceID: ref.InstanceId}, ref.Ref)
		} else {
			badVersions = true
			logging.Errorf(ctx, "Bad cached ref: %s: %s@%s: %s: instance does not exist on server",
				ref.Service, ref.Package, ref.Ref, ref.InstanceId)
		}
	}

	if badVersions {
		return cipderr.InvalidVersion.Apply(errors.New("had one or more bad cached versions"))
	}
	return nil
}

// CacheVerify will check the integrity of a cache directory.
//
// All instance files will have their hashes checked.
// All versions in the versioncache will be checked against the server. This
// means:
//   - Tags still point to the same instance ID that is cached.
//   - Refs still point to *some* instance ID which is present on the server.
//
// The discrepancy between tags and refs has to do with their immutability. It
// is expected that tags are immutable, and that refs can (and do) change over
// time. We just want to verify that this cache is plausibly correct, and don't
// currently have a way to verify that the instance the cached ref points to
// was "at some point in time" the value of the ref.
//
// Note that currently, the *only* caches which can even contain refs are ones
// prepared with [CachePrepare]. Normal `cipd` operation with CIPD_CACHE_DIR
// does not populate refs in the VersionCache at all. If you want to 'refresh'
// refs, you can do so by using [CachePrepare] with the same ensure file inputs
// again on the cache directory. It could also be possible to give CachePrepare
// a mode to just refresh the cache 'in place' by using the version cache and
// instance cache as inputs to then re-touch all instances which are tagged,
// all instances which are not in the version cache (e.g. instances fetched
// from a VersionFile), then resolve and fetch all instances from the
// cached refs to get them up to date, and then optionally trim the whole thing.
//
// If the cache directory does not contain a versioncache, this performs no
// network operations.
//
// If `clean` is provided, this will attempt to remove corrupt versions and
// instances.
//
// This does not support cached file version entries and if verifying a cache
// with file entries this will print a warning. If `clean` is supplied then any
// cached file entries will be dropped.
func CacheVerify(ctx context.Context, opts ClientOptions, cacheDir string, clean bool) error {
	if !filepath.IsAbs(cacheDir) {
		return cipderr.BadArgument.Apply(errors.Fmt("CacheVerify: must be given an absolute path, got: %q", cacheDir))
	}

	// We do not want to consult *any* caches in the client when resolving refs.
	opts.CacheDir = ""
	//opts.ReadOnlyCacheDir = ""

	targetInstCache := &internal.ManagedInstanceCache{
		Caches: []*internal.InstanceCache{
			{
				FS:      fs.NewFileSystem(filepath.Join(cacheDir, instancesSubdir), ""),
				Fetcher: nil, // For emphasis :).
			},
		},

		// NOTE: With no Fetcher, so no actual 'downloads' will happen, but this
		// allows the hash verification to be handled in parallel.
		ParallelDownloads:          32,
		ParallelDownloadsFastStart: true,
	}
	if clean {
		// If we are cleaning, then we do want deletion of corrupt files and also
		// synchronization of state.db. We don't want any garbage collection though
		// (which could remove instances for age related reasons).
		targetInstCache.Caches[0].PassiveWritePolicy = internal.DisableGC
	} else {
		// We are not cleaning, don't write anything.
		targetInstCache.Caches[0].PassiveWritePolicy = internal.DisablePassiveWrites
	}
	defer targetInstCache.Close(ctx)

	iids, err := targetInstCache.Caches[0].AllInstanceIDs(ctx, true)
	if err != nil {
		return err
	}

	activities := &ui.ActivityGroup{}

	reqs := make([]*internal.InstanceRequest, 0, len(iids))
	for _, iid := range iids {
		verifyCtx, verifyDone := ui.NewActivity(ctx, activities, "verify")

		reqs = append(reqs, &internal.InstanceRequest{
			Context: verifyCtx,
			OpenAs:  internal.VerifiedInstance,
			Done:    verifyDone,
			Pin: common.Pin{
				// We will never fetch due to Fetcher: nil, so this will never apply.
				PackageName: "fake/package",
				InstanceID:  iid,
			},
		})
	}
	targetInstCache.RequestInstances(ctx, reqs)

	var errs []error
	for range reqs {
		rsp := targetInstCache.WaitInstance()
		if err := rsp.Err; err != nil {
			if clean && reader.IsCorruptionError(err) {
				if !internal.CorruptFileDeletionFailure.In(err) {
					// We can ignore this: the instance was successfully cleaned and the
					// logs show it was a mismatch.
					continue
				}
			}
			errs = append(errs, err)
		} else if err := rsp.Instance.Close(ctx, false); err != nil {
			// This error should never happen, cleaning or not.
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return validateVersionCache(ctx, opts, cacheDir, clean)
}

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
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/common"
)

func TestCacheVerify(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (ClientOptions, *mockedRepoClient, *mockedStorage, string, context.Context) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		ctx = (&gologger.LoggerConfig{Out: t.Output()}).Use(ctx)
		opts, _, repo, storage := mockedClientOpts(t)

		opts.Root = t.TempDir()
		opts.ParallelDownloads = 1

		return opts, repo, storage, t.TempDir(), ctx
	}

	const pkgName = "some/pkg"
	const latest = "latest"
	const serviceURL = "https://service.example.com"
	const tag = "key:value"

	body, testPin := buildTestInstance(pkgName, map[string]string{"file": "content"})
	iid := testPin.InstanceID

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		opts, _, _, cacheDir, ctx := setup(t)

		// Create a valid instance in cache.
		instanceFile := filepath.Join(cacheDir, instancesSubdir, iid)
		err := os.MkdirAll(filepath.Dir(instanceFile), 0755)
		assert.NoErr(t, err)
		err = os.WriteFile(instanceFile, body, 0644)
		assert.NoErr(t, err)

		err = CacheVerify(ctx, opts, cacheDir, false)
		assert.NoErr(t, err)
	})

	t.Run("corrupt", func(t *testing.T) {
		t.Parallel()
		opts, _, _, cacheDir, ctx := setup(t)

		// Create a valid zip but with different content, so hash won't match iid.
		bodyCorrupt, _ := buildTestInstance(pkgName, map[string]string{"file": "corrupt content"})
		instanceFile := filepath.Join(cacheDir, instancesSubdir, iid)
		err := os.MkdirAll(filepath.Dir(instanceFile), 0755)
		assert.NoErr(t, err)
		err = os.WriteFile(instanceFile, bodyCorrupt, 0644)
		assert.NoErr(t, err)

		err = CacheVerify(ctx, opts, cacheDir, false)
		assert.ErrIsLike(t, err, "hash mismatch")
	})

	t.Run("clean", func(t *testing.T) {
		t.Parallel()
		opts, _, _, cacheDir, ctx := setup(t)

		// Create a valid zip but with different content, so hash won't match iid.
		bodyCorrupt, _ := buildTestInstance(pkgName, map[string]string{"file": "corrupt content"})
		instanceFile := filepath.Join(cacheDir, instancesSubdir, iid)
		err := os.MkdirAll(filepath.Dir(instanceFile), 0755)
		assert.NoErr(t, err)
		err = os.WriteFile(instanceFile, bodyCorrupt, 0644)
		assert.NoErr(t, err)

		// Call CacheVerify with clean = true
		err = CacheVerify(ctx, opts, cacheDir, true)
		assert.NoErr(t, err)

		// Verify that the corrupt file was removed
		_, err = os.Stat(instanceFile)
		assert.ErrIsLike(t, err, os.ErrNotExist)
	})

	t.Run("version_cache_ok", func(t *testing.T) {
		t.Parallel()
		opts, repo, _, cacheDir, ctx := setup(t)

		// Populate a version cache.
		cdfs := fs.NewFileSystem(cacheDir, "")
		vc := &internal.VersionCache{FS: cdfs}
		pin := common.Pin{PackageName: pkgName, InstanceID: iid}
		vc.AddVersion(ctx, serviceURL, pin, tag)
		err := vc.Flush(ctx)
		assert.NoErr(t, err)

		// Mock ResolveVersion for the version cache validation.
		repo.expect(rpcCall{
			method: "ResolveVersion",
			in:     &repopb.ResolveVersionRequest{Package: pkgName, Version: tag},
			out:    &repopb.Instance{Package: pkgName, Instance: common.InstanceIDToObjectRef(iid)},
		})

		err = CacheVerify(ctx, opts, cacheDir, false)
		assert.NoErr(t, err)
	})

	t.Run("version_cache_mismatch", func(t *testing.T) {
		t.Parallel()
		opts, repo, _, cacheDir, ctx := setup(t)

		// Populate a version cache.
		cdfs := fs.NewFileSystem(cacheDir, "")
		vc := &internal.VersionCache{FS: cdfs}
		pin := common.Pin{PackageName: pkgName, InstanceID: iid}
		vc.AddVersion(ctx, serviceURL, pin, tag)
		err := vc.Flush(ctx)
		assert.NoErr(t, err)

		// Mock ResolveVersion to return a DIFFERENT instance ID.
		otherIID := fakeIID("b")
		repo.expect(rpcCall{
			method: "ResolveVersion",
			in:     &repopb.ResolveVersionRequest{Package: pkgName, Version: tag},
			out:    &repopb.Instance{Package: pkgName, Instance: common.InstanceIDToObjectRef(otherIID)},
		})

		err = CacheVerify(ctx, opts, cacheDir, false)
		assert.ErrIsLike(t, err, "had one or more bad cached versions")
	})

	t.Run("version_cache_ref_ok", func(t *testing.T) {
		t.Parallel()
		opts, repo, _, cacheDir, ctx := setup(t)

		// Populate a version cache with a ref.
		cdfs := fs.NewFileSystem(cacheDir, "")
		vc := &internal.VersionCache{FS: cdfs}
		pin := common.Pin{PackageName: pkgName, InstanceID: iid}
		vc.AddVersion(ctx, serviceURL, pin, latest)
		err := vc.Flush(ctx)
		assert.NoErr(t, err)

		// Mock DescribeInstance for the version cache validation.
		repo.expect(rpcCall{
			method: "DescribeInstance",
			in: &repopb.DescribeInstanceRequest{
				Package:  pkgName,
				Instance: common.InstanceIDToObjectRef(iid),
			},
			out: &repopb.DescribeInstanceResponse{
				Instance: &repopb.Instance{
					Package:  pkgName,
					Instance: common.InstanceIDToObjectRef(iid),
				},
			},
		})

		// Refs are only checked when clean = true.
		err = CacheVerify(ctx, opts, cacheDir, true)
		assert.NoErr(t, err)

		// Verify that the ref is still in the cache.
		verCache, err := internal.ReadVersionCache(ctx, cdfs)
		assert.NoErr(t, err)
		found := false
		for _, ref := range verCache.RefEntries {
			if ref.Ref == latest && ref.Package == pkgName && ref.InstanceId == iid {
				found = true
				break
			}
		}
		assert.That(t, found, should.BeTrue)
	})

	t.Run("version_cache_ref_missing", func(t *testing.T) {
		t.Parallel()
		opts, repo, _, cacheDir, ctx := setup(t)

		// Populate a version cache with a ref.
		cdfs := fs.NewFileSystem(cacheDir, "")
		vc := &internal.VersionCache{FS: cdfs}
		pin := common.Pin{PackageName: pkgName, InstanceID: iid}
		vc.AddVersion(ctx, serviceURL, pin, latest)
		err := vc.Flush(ctx)
		assert.NoErr(t, err)

		// Mock DescribeInstance to fail with NotFound.
		repo.expect(rpcCall{
			method: "DescribeInstance",
			in: &repopb.DescribeInstanceRequest{
				Package:  pkgName,
				Instance: common.InstanceIDToObjectRef(iid),
			},
			err: status.Errorf(codes.NotFound, "no such instance"),
		})

		err = CacheVerify(ctx, opts, cacheDir, true)
		assert.ErrIsLike(t, err, "had one or more bad cached versions")

		// Verify that the ref is dropped from the cache.
		verCache, err := internal.ReadVersionCache(ctx, cdfs)
		assert.NoErr(t, err)
		found := false
		for _, ref := range verCache.GetRefEntries() {
			if ref.Ref == latest && ref.Package == pkgName {
				found = true
				break
			}
		}
		assert.That(t, found, should.BeFalse)
	})

	t.Run("version_cache_tag_missing", func(t *testing.T) {
		t.Parallel()
		opts, repo, _, cacheDir, ctx := setup(t)

		const tag = "key:value"

		// Populate a version cache with a tag.
		cdfs := fs.NewFileSystem(cacheDir, "")
		vc := &internal.VersionCache{FS: cdfs}
		pin := common.Pin{PackageName: pkgName, InstanceID: iid}
		vc.AddVersion(ctx, serviceURL, pin, tag)
		err := vc.Flush(ctx)
		assert.NoErr(t, err)

		// Mock ResolveVersion to fail with NotFound.
		repo.expect(rpcCall{
			method: "ResolveVersion",
			in:     &repopb.ResolveVersionRequest{Package: pkgName, Version: tag},
			err:    status.Errorf(codes.NotFound, "no such package"),
		})

		err = CacheVerify(ctx, opts, cacheDir, true)
		assert.ErrIsLike(t, err, "had one or more bad cached versions")

		// Verify that the tag is dropped from the cache.
		verCache, err := internal.ReadVersionCache(ctx, cdfs)
		assert.NoErr(t, err)
		found := false
		for _, entry := range verCache.GetEntries() {
			if entry.Tag == tag && entry.Package == pkgName {
				found = true
				break
			}
		}
		assert.That(t, found, should.BeFalse)
	})

	t.Run("version_cache_mismatch_clean", func(t *testing.T) {
		t.Parallel()
		opts, repo, _, cacheDir, ctx := setup(t)

		const tag = "key:value"

		// Populate a version cache with a tag pointing to iid.
		cdfs := fs.NewFileSystem(cacheDir, "")
		vc := &internal.VersionCache{FS: cdfs}
		pin := common.Pin{PackageName: pkgName, InstanceID: iid}
		vc.AddVersion(ctx, serviceURL, pin, tag)
		err := vc.Flush(ctx)
		assert.NoErr(t, err)

		// Mock ResolveVersion to return a DIFFERENT instance ID.
		otherIID := fakeIID("b")
		repo.expect(rpcCall{
			method: "ResolveVersion",
			in:     &repopb.ResolveVersionRequest{Package: pkgName, Version: tag},
			out:    &repopb.Instance{Package: pkgName, Instance: common.InstanceIDToObjectRef(otherIID)},
		})

		// Call CacheVerify with clean = true
		err = CacheVerify(ctx, opts, cacheDir, true)
		// It should still return error because it found a bad version during check.
		assert.ErrIsLike(t, err, "had one or more bad cached versions")

		// Verify that the bad mapping is dropped entirely.
		verCache, err := internal.ReadVersionCache(ctx, cdfs)
		assert.NoErr(t, err)

		found := false
		for _, entry := range verCache.GetEntries() {
			if entry.Tag == tag && entry.Package == pkgName {
				found = true
				break
			}
		}
		assert.That(t, found, should.BeFalse)
	})
}

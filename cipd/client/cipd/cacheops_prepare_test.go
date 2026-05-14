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

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common"
)

func TestCachePrepare(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (ClientOptions, *mockedRepoClient, *mockedStorage, string, context.Context) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		_ = tc // Ignore unused variable
		ctx = (&gologger.LoggerConfig{Out: t.Output()}).Use(ctx)
		opts, _, repo, storage := mockedClientOpts(t)

		// We need to set a root directory for the client, but CachePrepare
		// will create its own cache directory.
		opts.Root = t.TempDir()
		opts.ParallelDownloads = 1

		return opts, repo, storage, t.TempDir(), ctx
	}

	const pkgName = "some/pkg"
	const pkgURL = "https://example.com/dl/" + pkgName
	const latest = "latest"
	const serviceURL = "https://service.example.com"

	body, testPin := buildTestInstance(pkgName, map[string]string{"file": "content"})

	iid := testPin.InstanceID
	objRef := common.InstanceIDToObjectRef(iid)

	setResolve := func(repo *mockedRepoClient) {
		repo.expect(rpcCall{
			method: "ResolveVersion",
			in:     &repopb.ResolveVersionRequest{Package: pkgName, Version: latest},
			out:    &repopb.Instance{Package: pkgName, Instance: objRef},
		})
		repo.expect(rpcCall{
			method: "GetInstanceURL",
			in:     &repopb.GetInstanceURLRequest{Package: pkgName, Instance: objRef},
			out:    &caspb.ObjectURL{SignedUrl: pkgURL},
		})
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		opts, repo, storage, cacheDir, ctx := setup(t)

		setResolve(repo)
		storage.putStored(pkgURL, string(body))

		versions := PackageVersionSet{}
		versions.Add(serviceURL, pkgName, latest)

		err := CachePrepare(ctx, opts, cacheDir, false, versions)
		assert.NoErr(t, err)

		// Verify that the file was fetched into the cache directory.
		// The cache directory structure is: cacheDir/instances/iid
		expectedFile := filepath.Join(cacheDir, instancesSubdir, iid)
		_, err = os.Stat(expectedFile)
		assert.NoErr(t, err)
	})

	t.Run("trim", func(t *testing.T) {
		t.Parallel()
		opts, repo, storage, cacheDir, ctx := setup(t)

		setResolve(repo)
		storage.putStored(pkgURL, string(body))

		trimmedIID := fakeIID("b")
		trimmedFile := filepath.Join(cacheDir, instancesSubdir, trimmedIID)
		err := os.MkdirAll(filepath.Dir(trimmedFile), 0755)
		assert.NoErr(t, err)
		err = os.WriteFile(trimmedFile, []byte("garbage content"), 0644)
		assert.NoErr(t, err)

		versions := PackageVersionSet{}
		versions.Add(serviceURL, pkgName, latest)

		err = CachePrepare(ctx, opts, cacheDir, true, versions)
		assert.NoErr(t, err)

		expectedFile := filepath.Join(cacheDir, instancesSubdir, iid)
		_, err = os.Stat(expectedFile)
		assert.NoErr(t, err)

		_, err = os.Stat(trimmedFile)
		assert.ErrIsLike(t, err, os.ErrNotExist)
	})

	t.Run("no_resolve", func(t *testing.T) {
		t.Parallel()
		opts, repo, _, cacheDir, ctx := setup(t)

		repo.expect(rpcCall{
			method: "ResolveVersion",
			in:     &repopb.ResolveVersionRequest{Package: pkgName, Version: latest},
			err:    status.Errorf(codes.NotFound, "no such package"),
		})

		versions := PackageVersionSet{}
		serviceURL := "https://service.example.com"
		versions.Add(serviceURL, pkgName, latest)

		err := CachePrepare(ctx, opts, cacheDir, false, versions)
		assert.ErrIsLike(t, err, "no such package")
	})
}

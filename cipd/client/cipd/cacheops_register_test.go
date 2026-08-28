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

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
)

func TestCacheRegister(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (ClientOptions, string, context.Context) {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		ctx = (&gologger.LoggerConfig{Out: t.Output()}).Use(ctx)
		opts, _, _, _ := mockedClientOpts(t)

		cacheDir := t.TempDir()
		opts.Root = t.TempDir()
		opts.ReadOnlyCacheDir = cacheDir
		opts.ServiceURL = "https://service.example.com"

		return opts, cacheDir, ctx
	}

	const pkgName = "some/pkg"
	const serviceURL = "https://service.example.com"

	body, testPin := buildTestInstance(pkgName, map[string]string{"file": "content"})
	iid := testPin.InstanceID

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		opts, cacheDir, ctx := setup(t)

		src := pkg.NewBytesSource(body)
		tags := []string{"key:value", "version:1.0.0"}
		refs := []string{"latest"}

		err := CacheRegister(ctx, opts.ServiceURL, cacheDir, src, testPin, tags, refs)
		assert.NoErr(t, err)

		// Verify instance file was written to <cacheDir>/instances/<iid>.
		instanceFile := filepath.Join(cacheDir, instancesSubdir, iid)
		content, err := os.ReadFile(instanceFile)
		assert.NoErr(t, err)
		assert.That(t, content, should.Match(body))

		// Verify tags and refs in VersionCache.
		vc := &internal.VersionCache{
			FS: fs.NewFileSystem(cacheDir, ""),
		}

		pin, err := vc.ResolveTag(ctx, serviceURL, pkgName, "key:value")
		assert.NoErr(t, err)
		assert.That(t, pin, should.Match(testPin))

		pin, err = vc.ResolveTag(ctx, serviceURL, pkgName, "version:1.0.0")
		assert.NoErr(t, err)
		assert.That(t, pin, should.Match(testPin))

		pin, err = vc.ResolveRef(ctx, serviceURL, pkgName, "latest")
		assert.NoErr(t, err)
		assert.That(t, pin, should.Match(testPin))
	})

	t.Run("bad args", func(t *testing.T) {
		t.Parallel()
		opts, cacheDir, ctx := setup(t)

		src := pkg.NewBytesSource(body)

		// Relative cacheDir
		err := CacheRegister(ctx, opts.ServiceURL, "relative/path", src, testPin, nil, nil)
		assert.ErrIsLike(t, err, "must be an absolute path")

		// Nil source
		err = CacheRegister(ctx, opts.ServiceURL, cacheDir, nil, testPin, nil, nil)
		assert.ErrIsLike(t, err, "Source is required")

		// Invalid tag
		err = CacheRegister(ctx, opts.ServiceURL, cacheDir, src, testPin, []string{"not_a_tag"}, nil)
		assert.Loosely(t, err, should.NotBeNil)

		// Invalid ref
		err = CacheRegister(ctx, opts.ServiceURL, cacheDir, src, testPin, nil, []string{"invalid ref!"})
		assert.Loosely(t, err, should.NotBeNil)
	})

	t.Run("offline resolution with CIPD client", func(t *testing.T) {
		t.Parallel()
		opts, cacheDir, ctx := setup(t)

		src := pkg.NewBytesSource(body)
		err := CacheRegister(ctx, opts.ServiceURL, cacheDir, src, testPin, []string{"version:2.0.0"}, []string{"latest"})
		assert.NoErr(t, err)

		// Create an offline client pointing to the read-only cache.
		clientOpts := opts
		clientOpts.ReadOnlyCacheDir = cacheDir
		clientOpts.DisableNetwork = true

		client, err := NewClient(clientOpts)
		assert.NoErr(t, err)
		defer client.Close(ctx)

		// Resolve tag offline.
		pin, err := client.ResolveVersion(ctx, pkgName, "version:2.0.0")
		assert.NoErr(t, err)
		assert.That(t, pin, should.Match(testPin))

		// Resolve ref offline.
		pin, err = client.ResolveVersion(ctx, pkgName, "latest")
		assert.NoErr(t, err)
		assert.That(t, pin, should.Match(testPin))

		// Resolve concrete instance ID offline.
		pin, err = client.ResolveVersion(ctx, pkgName, iid)
		assert.NoErr(t, err)
		assert.That(t, pin, should.Match(testPin))
	})
}

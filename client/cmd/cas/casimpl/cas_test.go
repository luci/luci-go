// Copyright 2020 The LUCI Authors.
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

package casimpl

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"

	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/data/embeddedkvs"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/testfs"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testAuthFlags struct {
	testEnv *fakes.TestEnv
}

func (af *testAuthFlags) Register(_ *flag.FlagSet) {}

func (af *testAuthFlags) Parse() error { return nil }

func (af *testAuthFlags) NewRBEClient(ctx context.Context, _ string, _ string, _ bool) (*client.Client, error) {
	return af.testEnv.Server.NewTestClient(ctx)
}

func TestArchiveDownload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run(`Upload and download`, t, func(t *ftt.Test) {
		testEnv, cleanup := fakes.NewTestEnv(t)
		t.Cleanup(cleanup)

		uploaded := t.TempDir()
		largeFile := string(make([]byte, smallFileThreshold+1))
		layout := map[string]string{
			"a":          "a",
			"b/c":        "bc",
			"large_file": largeFile,
			"empty/":     "",
		}
		assert.Loosely(t, testfs.Build(uploaded, layout), should.BeNil)

		var ar archiveRun
		ar.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
		ar.dumpDigest = filepath.Join(t.TempDir(), "digest")
		assert.Loosely(t, ar.paths.Set(uploaded+":."), should.BeNil)
		assert.Loosely(t, ar.doArchive(ctx), should.BeNil)

		digest, err := os.ReadFile(ar.dumpDigest)
		assert.Loosely(t, err, should.BeNil)

		var dr downloadRun
		dr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
		dr.digest = string(digest)
		dr.dir = t.TempDir()

		check := func(t testing.TB) {
			t.Helper()
			downloaded, err := testfs.Collect(dr.dir)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			assert.Loosely(t, downloaded, should.Match(layout), truth.LineContext())
		}

		t.Run("use cache", func(t *ftt.Test) {
			dr.kvs = filepath.Join(t.TempDir(), "kvs")
			dr.cacheDir = filepath.Join(t.TempDir(), "cache")
			err = dr.doDownload(ctx)
			assert.Loosely(t, err, should.BeNil)

			kvs, err := embeddedkvs.New(ctx, dr.kvs)
			assert.Loosely(t, err, should.BeNil)
			defer func() {
				assert.Loosely(t, kvs.Close(), should.BeNil)
			}()

			var keys, values []string
			assert.Loosely(t, kvs.ForEach(func(key string, value []byte) error {
				keys = append(keys, key)
				values = append(values, string(value))
				return nil
			}), should.BeNil)

			sha256hex := func(value string) string {
				h := sha256.Sum256([]byte(value))
				return hex.EncodeToString(h[:])
			}

			assert.Loosely(t, keys, should.Match([]string{
				sha256hex("bc"),
				sha256hex("a"),
			}))
			assert.Loosely(t, values, should.Match([]string{"bc", "a"}))

			diskcache, err := cache.New(dr.cachePolicies, dr.cacheDir, crypto.SHA256)
			assert.Loosely(t, err, should.BeNil)
			defer func() {
				assert.Loosely(t, diskcache.Close(), should.BeNil)
			}()
			assert.Loosely(t, diskcache.Touch(cache.HexDigest(sha256hex(largeFile))), should.BeTrue)
			check(t)
		})

		t.Run("not use cache", func(t *ftt.Test) {
			// do not set kvs.
			err = dr.doDownload(ctx)
			assert.Loosely(t, err, should.BeNil)
			check(t)
		})
	})
}

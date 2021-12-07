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
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/data/embeddedkvs"
	"go.chromium.org/luci/common/testing/testfs"
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

	Convey(`Upload and download`, t, func() {
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
		So(testfs.Build(uploaded, layout), ShouldBeNil)

		var ar archiveRun
		ar.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
		ar.dumpDigest = filepath.Join(t.TempDir(), "digest")
		So(ar.paths.Set(uploaded+":."), ShouldBeNil)
		So(ar.doArchive(ctx), ShouldBeNil)

		digest, err := ioutil.ReadFile(ar.dumpDigest)
		So(err, ShouldBeNil)

		var dr downloadRun
		dr.commonFlags.Init(&testAuthFlags{testEnv: testEnv})
		dr.digest = string(digest)
		dr.dir = t.TempDir()

		Convey("use cache", func() {
			dr.kvs = filepath.Join(t.TempDir(), "kvs")
			dr.cacheDir = filepath.Join(t.TempDir(), "cache")
			err = dr.doDownload(ctx)
			So(err, ShouldBeNil)

			kvs, err := embeddedkvs.New(ctx, dr.kvs)
			So(err, ShouldBeNil)
			defer func() {
				So(kvs.Close(), ShouldBeNil)
			}()

			var keys, values []string
			So(kvs.ForEach(func(key string, value []byte) error {
				keys = append(keys, key)
				values = append(values, string(value))
				return nil
			}), ShouldBeNil)

			sha256hex := func(value string) string {
				h := sha256.Sum256([]byte(value))
				return hex.EncodeToString(h[:])
			}

			So(keys, ShouldResemble, []string{
				sha256hex("bc"),
				sha256hex("a"),
			})
			So(values, ShouldResemble, []string{"bc", "a"})

			diskcache, err := cache.New(dr.cachePolicies, dr.cacheDir, crypto.SHA256)
			So(err, ShouldBeNil)
			defer func() {
				So(diskcache.Close(), ShouldBeNil)
			}()
			So(diskcache.Touch(cache.HexDigest(sha256hex(largeFile))), ShouldBeTrue)
		})

		Convey("not use cache", func() {
			// do not set kvs.
			err = dr.doDownload(ctx)
			So(err, ShouldBeNil)
		})

		downloaded, err := testfs.Collect(dr.dir)
		So(err, ShouldBeNil)
		So(downloaded, ShouldResemble, layout)
	})
}

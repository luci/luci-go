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

package lib

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/testing/testfs"
)

func TestArchiveDownload(t *testing.T) {
	ctx := context.Background()

	Convey(`Upload and download`, t, func() {
		fakeEnv, cleanup := fakes.NewTestEnv(t)
		t.Cleanup(cleanup)

		newCasClient = func(ctx context.Context, instance string, opts auth.Options, readOnly bool) (*client.Client, error) {
			return fakeEnv.Server.NewTestClient(ctx)
		}

		uploaded := t.TempDir()
		layout := map[string]string{
			"a":      "a",
			"b/c":    "bc",
			"empty/": "",
		}
		So(testfs.Build(uploaded, layout), ShouldBeNil)

		var ar archiveRun
		ar.dumpDigest = filepath.Join(t.TempDir(), "digest")
		So(ar.paths.Set(uploaded+":."), ShouldBeNil)
		So(ar.doArchive(ctx), ShouldBeNil)

		digest, err := ioutil.ReadFile(ar.dumpDigest)
		So(err, ShouldBeNil)

		var dr downloadRun
		dr.digest = string(digest)
		dr.dir = t.TempDir()
		err = dr.doDownload(ctx)
		So(err, ShouldBeNil)

		downloaded, err := testfs.Collect(dr.dir)
		So(err, ShouldBeNil)
		So(downloaded, ShouldResemble, layout)
	})
}

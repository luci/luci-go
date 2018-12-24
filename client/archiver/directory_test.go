// Copyright 2015 The LUCI Authors.
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

package archiver

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPushDirectory(t *testing.T) {
	// Uploads a real directory. 2 times the same file.
	t.Parallel()
	emptyContext := context.Background()
	fooDigest := isolated.HashBytes([]byte("foo"))

	Convey(`Pushing a real directory should upload the directory.`, t, func() {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()
		a := New(emptyContext, isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil), nil)

		// Setup temporary directory.
		tmpDir, err := ioutil.TempDir("", "archiver")
		So(err, ShouldBeNil)
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Fail()
			}
		}()
		baseDir := filepath.Join(tmpDir, "base")
		ignoredDir := filepath.Join(tmpDir, "ignored1")
		So(os.Mkdir(baseDir, 0700), ShouldBeNil)
		So(ioutil.WriteFile(filepath.Join(baseDir, "bar"), []byte("foo"), 0600), ShouldBeNil)
		So(ioutil.WriteFile(filepath.Join(baseDir, "bar_dupe"), []byte("foo"), 0600), ShouldBeNil)
		outOfTreeFile, err := ioutil.TempFile("", "out-of-tree")
		So(err, ShouldBeNil)
		_, err = outOfTreeFile.Write([]byte("foo"))
		So(err, ShouldBeNil)
		defer outOfTreeFile.Close()

		if !common.IsWindows() {
			So(os.Symlink(filepath.Join(baseDir, "bar"), filepath.Join(baseDir, "link_inside")), ShouldBeNil)
			So(os.Symlink(outOfTreeFile.Name(), filepath.Join(baseDir, "link_outside")), ShouldBeNil)
		}
		So(ioutil.WriteFile(filepath.Join(baseDir, "ignored2"), []byte("ignored"), 0600), ShouldBeNil)
		So(os.Mkdir(ignoredDir, 0700), ShouldBeNil)
		So(ioutil.WriteFile(filepath.Join(ignoredDir, "really"), []byte("ignored"), 0600), ShouldBeNil)

		item := PushDirectory(a, tmpDir, "", []string{"ignored1", filepath.Join("*", "ignored2")})
		So(item.DisplayName, ShouldResemble, filepath.Base(tmpDir)+".isolated")
		item.WaitForHashed()
		So(a.Close(), ShouldBeNil)

		mode := 0600
		if common.IsWindows() {
			mode = 0666
		}
		basicFooFile := isolated.BasicFile(fooDigest, mode, 3)
		isolatedData := isolated.Isolated{
			Algo: "sha-1",
			Files: map[string]isolated.File{
				filepath.Join("base", "bar"):      basicFooFile,
				filepath.Join("base", "bar_dupe"): basicFooFile,
			},
			Version: isolated.IsolatedFormatVersion,
		}
		expectedMisses := 3                      // bar + bar_dupe + ignored
		expectedBytesPushed := units.Size(3 + 3) // "foo" + "foo"
		if !common.IsWindows() {
			isolatedData.Files[filepath.Join("base", "link_inside")] = isolated.SymLink(filepath.Join(baseDir, "bar"))
			info, err := os.Lstat(filepath.Join(baseDir, "link_outside"))
			mode := info.Mode()
			So(err, ShouldBeNil)
			isolatedData.Files[filepath.Join("base", "link_outside")] = isolated.BasicFile(fooDigest, int(mode.Perm()), info.Size())
			expectedMisses += 1
			expectedBytesPushed += units.Size(3)
		}
		encoded, err := json.Marshal(isolatedData)
		So(err, ShouldBeNil)
		isolatedEncoded := string(encoded) + "\n"
		isolatedHash := isolated.HashBytes([]byte(isolatedEncoded))
		expectedBytesPushed += units.Size(len(isolatedEncoded))

		expected := map[string]string{
			string(fooDigest):    "foo",
			string(isolatedHash): isolatedEncoded,
		}
		actual := map[string]string{}
		for k, v := range server.Contents() {
			actual[string(k)] = string(v)
		}
		So(actual, ShouldResemble, expected)
		So(item.Digest(), ShouldResemble, isolatedHash)

		stats := a.Stats()
		So(stats.TotalHits(), ShouldResemble, 0)
		// There're 4 cache misses even if the same content is looked up twice.
		So(stats.TotalMisses(), ShouldResemble, expectedMisses)
		So(stats.TotalBytesHits(), ShouldResemble, units.Size(0))
		So(stats.TotalBytesPushed(), ShouldResemble, expectedBytesPushed)

		So(server.Error(), ShouldBeNil)
	})
}

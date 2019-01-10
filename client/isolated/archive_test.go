// Copyright 2017 The LUCI Authors.
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

package isolated

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArchive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`Tests the archival of some files.`, t, func() {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()

		// Setup temporary directory.
		//   /base/bar
		//   /base/ignored
		//   /link -> /base/bar
		//   /second/boz
		// Result:
		//   /baz.isolated
		tmpDir, err := ioutil.TempDir("", "isolated")
		So(err, ShouldBeNil)
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Fail()
			}
		}()
		baseDir := filepath.Join(tmpDir, "base")
		fooDir := filepath.Join(tmpDir, "foo")
		secondDir := filepath.Join(tmpDir, "second")
		So(os.Mkdir(baseDir, 0700), ShouldBeNil)
		So(os.Mkdir(fooDir, 0700), ShouldBeNil)
		So(os.Mkdir(secondDir, 0700), ShouldBeNil)

		barData := []byte("foo")
		ignoredData := []byte("ignored")
		bozData := []byte("foo2")
		So(ioutil.WriteFile(filepath.Join(baseDir, "bar"), barData, 0600), ShouldBeNil)
		So(ioutil.WriteFile(filepath.Join(baseDir, "ignored"), ignoredData, 0600), ShouldBeNil)
		So(ioutil.WriteFile(filepath.Join(secondDir, "boz"), bozData, 0600), ShouldBeNil)
		winLinkData := []byte("no link on Windows")
		if !isWindows() {
			So(os.Symlink(filepath.Join("base", "bar"), filepath.Join(tmpDir, "link")), ShouldBeNil)
		} else {
			So(ioutil.WriteFile(filepath.Join(tmpDir, "link"), winLinkData, 0600), ShouldBeNil)
		}

		var buf bytes.Buffer
		filesSC := ScatterGather{}
		So(filesSC.Add(tmpDir, "link"), ShouldBeNil)
		dirsSC := ScatterGather{}
		So(dirsSC.Add(tmpDir, "base"), ShouldBeNil)
		So(dirsSC.Add(tmpDir, "second"), ShouldBeNil)

		opts := &ArchiveOptions{
			Files:        filesSC,
			Dirs:         dirsSC,
			Blacklist:    []string{"ignored", "*.isolate"},
			Isolated:     filepath.Join(tmpDir, "baz.isolated"),
			LeakIsolated: &buf,
		}
		a := archiver.New(ctx, isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil), nil)
		item := Archive(ctx, a, opts)
		So(item.DisplayName, ShouldResemble, filepath.Join(tmpDir, "baz.isolated"))
		item.WaitForHashed()
		So(item.Error(), ShouldBeNil)
		So(a.Close(), ShouldBeNil)

		mode := 0600
		if isWindows() {
			mode = 0666
		}

		baseData := oneFileArchiveExpect(filepath.Join("base", "bar"), mode, barData)
		secondData := oneFileArchiveExpect(filepath.Join("second", "boz"), mode, bozData)
		topIsolated := isolated.New()
		topIsolated.Includes = isolated.HexDigests{baseData.Hash, secondData.Hash}
		if !isWindows() {
			topIsolated.Files["link"] = isolated.BasicFile(isolated.HashBytes(barData), mode, int64(len(barData)))
		} else {
			topIsolated.Files["link"] = isolated.BasicFile(isolated.HashBytes(winLinkData), 0666, int64(len(winLinkData)))
		}
		isolatedData := newArchiveExpectData(topIsolated)
		expected := map[isolated.HexDigest]string{
			isolated.HashBytes(barData): string(barData),
			isolated.HashBytes(bozData): string(bozData),
			baseData.Hash:               baseData.String,
			isolatedData.Hash:           isolatedData.String,
			secondData.Hash:             secondData.String,
		}
		if isWindows() {
			expected[isolated.HashBytes(winLinkData)] = string(winLinkData)
		}
		actual := map[isolated.HexDigest]string{}
		for k, v := range server.Contents() {
			actual[isolated.HexDigest(k)] = string(v)
			So(actual[isolated.HexDigest(k)], ShouldResemble, expected[isolated.HexDigest(k)])
		}
		So(actual, ShouldResemble, expected)
		So(item.Digest(), ShouldResemble, isolatedData.Hash)
		So(server.Error(), ShouldBeNil)
		digest := isolated.HashBytes(buf.Bytes())
		So(digest, ShouldResemble, isolatedData.Hash)
	})
}

func isWindows() bool {
	return runtime.GOOS == "windows"
}

type archiveExpectData struct {
	*isolated.Isolated
	String string
	Hash   isolated.HexDigest
}

func oneFileArchiveExpect(file string, mode int, data []byte) *archiveExpectData {
	i := isolated.New()
	i.Files[file] = isolated.BasicFile(isolated.HashBytes(data), mode, int64(len(data)))
	return newArchiveExpectData(i)
}

func newArchiveExpectData(i *isolated.Isolated) *archiveExpectData {
	encoded, _ := json.Marshal(i)
	str := string(encoded) + "\n"
	return &archiveExpectData{
		Isolated: i,
		String:   str,
		Hash:     isolated.HashBytes([]byte(str)),
	}
}

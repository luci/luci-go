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

package downloader

import (
	"encoding/json"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDownloaderFetchIsolated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	string1 := "hello world!"
	data1 := []byte(string1)

	string2 := "wat"
	data2 := []byte(string2)

	server := isolatedfake.New()
	data1hash := server.Inject(data1)
	data2hash := server.Inject(data2)

	isolated1 := isolated.New()
	isolated1.Files = map[string]isolated.File{
		"foo/one.txt": isolated.BasicFile(data1hash, 0664, int64(len(data1))),
		"foo/two.txt": isolated.BasicFile(data2hash, 0664, int64(len(data2))),
	}
	isolated1bytes, _ := json.Marshal(&isolated1)
	isolated1hash := server.Inject(isolated1bytes)

	isolated2 := isolated.New()
	isolated2.Files = map[string]isolated.File{
		"bar/lol.txt":     isolated.BasicFile(data1hash, 0664, int64(len(data1))),
		"foo/boz/olo.txt": isolated.BasicFile(data2hash, 0664, int64(len(data2))),
	}
	// Symlinks not supported on Windows.
	if runtime.GOOS != "windows" {
		isolated2.Files["blah.txt"] = isolated.SymLink("foo/boz/olo.txt")
	}
	isolated2.Includes = isolated.HexDigests{isolated1hash}
	isolated2bytes, _ := json.Marshal(&isolated2)
	isolated2hash := server.Inject(isolated2bytes)

	ts := httptest.NewServer(server)
	defer ts.Close()
	client := isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil)

	Convey(`A downloader should be able to download the isolated.`, t, func() {
		tmpDir, err := ioutil.TempDir("", "isolated")
		So(err, ShouldBeNil)

		d := New(ctx, client, 8)
		err = d.FetchIsolated(isolated2hash, tmpDir)
		So(err, ShouldBeNil)

		oneBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "foo/one.txt"))
		So(err, ShouldBeNil)
		So(oneBytes, ShouldResemble, data1)

		twoBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "foo/two.txt"))
		So(err, ShouldBeNil)
		So(twoBytes, ShouldResemble, data2)

		lolBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "bar/lol.txt"))
		So(err, ShouldBeNil)
		So(lolBytes, ShouldResemble, data1)

		oloBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, "foo/boz/olo.txt"))
		So(err, ShouldBeNil)
		So(oloBytes, ShouldResemble, data2)

		if runtime.GOOS != "windows" {
			l, err := os.Readlink(filepath.Join(tmpDir, "blah.txt"))
			So(err, ShouldBeNil)
			So(l, ShouldResemble, filepath.Join(tmpDir, "foo/boz/olo.txt"))
		}
	})
}

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

	"go.chromium.org/luci/common/data/stringset"
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

	onePath := filepath.Join("foo", "one.txt")
	twoPath := filepath.Join("foo", "two.txt")
	isolated1 := isolated.New()
	isolated1.Files = map[string]isolated.File{
		onePath: isolated.BasicFile(data1hash, 0664, int64(len(data1))),
		twoPath: isolated.BasicFile(data2hash, 0664, int64(len(data2))),
	}
	isolated1bytes, _ := json.Marshal(&isolated1)
	isolated1hash := server.Inject(isolated1bytes)

	lolPath := filepath.Join("bar", "lol.txt")
	oloPath := filepath.Join("foo", "boz", "olo.txt")
	isolated2 := isolated.New()
	isolated2.Files = map[string]isolated.File{
		lolPath: isolated.BasicFile(data1hash, 0664, int64(len(data1))),
		oloPath: isolated.BasicFile(data2hash, 0664, int64(len(data2))),
	}
	isolatedFiles := stringset.NewFromSlice([]string{
		onePath,
		twoPath,
		lolPath,
		oloPath,
	}...)
	blahPath := "blah.txt"

	// Symlinks not supported on Windows.
	if runtime.GOOS != "windows" {
		isolated2.Files[blahPath] = isolated.SymLink(oloPath)
		isolatedFiles.Add(blahPath)
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
		files, err := d.FetchIsolated(isolated2hash, tmpDir)
		So(err, ShouldBeNil)
		So(stringset.NewFromSlice(files...), ShouldResemble, isolatedFiles)

		oneBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, onePath))
		So(err, ShouldBeNil)
		So(oneBytes, ShouldResemble, data1)

		twoBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, twoPath))
		So(err, ShouldBeNil)
		So(twoBytes, ShouldResemble, data2)

		lolBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, lolPath))
		So(err, ShouldBeNil)
		So(lolBytes, ShouldResemble, data1)

		oloBytes, err := ioutil.ReadFile(filepath.Join(tmpDir, oloPath))
		So(err, ShouldBeNil)
		So(oloBytes, ShouldResemble, data2)

		if runtime.GOOS != "windows" {
			l, err := os.Readlink(filepath.Join(tmpDir, blahPath))
			So(err, ShouldBeNil)
			So(l, ShouldResemble, filepath.Join(tmpDir, oloPath))
		}
	})
}

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
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"

	. "github.com/smartystreets/goconvey/convey"
)

type dummyFile struct {
	bytes.Buffer
}

func (d *dummyFile) Write(p []byte) (int, error) {
	return d.Buffer.Write(p)
}

func (_ *dummyFile) Close() error {
	return nil
}

func TestDownloaderEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`An empty downloader should do nothing.`, t, func() {
		client := isolatedclient.New(nil, nil, "https://localhost:1", isolatedclient.DefaultNamespace, nil, nil)
		d := New(ctx, client)
		So(d.Close(), ShouldBeNil)
	})
}

func TestDownloaderFetchFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	string1 := "hello world!"
	data1 := []byte(string1)

	string2 := "wat"
	data2 := []byte(string2)

	server := isolatedfake.New()
	data1hash := server.Inject(data1)
	data2hash := server.Inject(data2)
	ts := httptest.NewServer(server)
	defer ts.Close()
	client := isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil)

	Convey(`A downloader should be able to download a file.`, t, func() {
		var buffer dummyFile
		d := New(ctx, client)
		d.FetchFile(data1hash, &buffer)
		So(d.Close(), ShouldBeNil)
		So(buffer.String(), ShouldResemble, string1)
	})

	Convey(`A downloader should be able to download many files.`, t, func() {
		var buffer1 dummyFile
		var buffer2 dummyFile

		d := New(ctx, client)
		d.FetchFile(data1hash, &buffer1)
		d.FetchFile(data2hash, &buffer2)
		So(d.Close(), ShouldBeNil)

		So(buffer1.String(), ShouldResemble, string1)
		So(buffer2.String(), ShouldResemble, string2)
	})
}

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
		"./one.txt": isolated.BasicFile(data1hash, 0664, int64(len(data1))),
		"./two.txt": isolated.BasicFile(data2hash, 0664, int64(len(data2))),
	}
	isolated1bytes, _ := json.Marshal(&isolated1)
	isolated1hash := server.Inject(isolated1bytes)

	isolated2 := isolated.New()
	isolated2.Files = map[string]isolated.File{
		"./lol.txt": isolated.BasicFile(data1hash, 0664, int64(len(data1))),
		"./olo.txt": isolated.BasicFile(data2hash, 0664, int64(len(data2))),
	}
	isolated2.Includes = isolated.HexDigests{isolated1hash}
	isolated2bytes, _ := json.Marshal(&isolated2)
	isolated2hash := server.Inject(isolated2bytes)

	ts := httptest.NewServer(server)
	defer ts.Close()
	client := isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil)

	Convey(`A downloader should be able to download the isolated as a file.`, t, func() {
		var buffer dummyFile
		d := New(ctx, client)
		d.FetchFile(isolated1hash, &buffer)
		So(d.Close(), ShouldBeNil)
		result := isolated.New()
		So(json.Unmarshal(buffer.Bytes(), result), ShouldBeNil)
		So(result, ShouldResemble, isolated1)
	})

	Convey(`A downloader should be able to download the isolated.`, t, func() {
		registry := map[string]*dummyFile{}
		d := New(ctx, client)
		d.FetchIsolated(isolated2hash, func(name string) (io.WriteCloser, error) {
			var buf dummyFile
			registry[name] = &buf
			return &buf, nil
		})
		So(d.Close(), ShouldBeNil)
		So(registry["./one.txt"].String(), ShouldResemble, string1)
		So(registry["./two.txt"].String(), ShouldResemble, string2)
		So(registry["./lol.txt"].String(), ShouldResemble, string1)
		So(registry["./olo.txt"].String(), ShouldResemble, string2)
	})
}

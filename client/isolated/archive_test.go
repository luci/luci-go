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
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
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
		// TODO(maruel): Make other algorithms work too.
		for _, namespace := range []string{isolatedclient.DefaultNamespace} {
			Convey(fmt.Sprintf("Run on namespace %s", namespace), func() {
				a := archiver.New(ctx, isolatedclient.New(nil, nil, ts.URL, namespace, nil, nil), nil)
				item := Archive(ctx, a, opts)
				So(item.DisplayName, ShouldResemble, filepath.Join(tmpDir, "baz.isolated"))
				item.WaitForHashed()
				So(item.Error(), ShouldBeNil)
				So(a.Close(), ShouldBeNil)

				mode := 0600
				if isWindows() {
					mode = 0666
				}

				h := isolated.GetHash(namespace)
				baseData := oneFileArchiveExpect(h, filepath.Join("base", "bar"), mode, barData)
				secondData := oneFileArchiveExpect(h, filepath.Join("second", "boz"), mode, bozData)
				topIsolated := isolated.New(h)
				topIsolated.Includes = isolated.HexDigests{baseData.Hash, secondData.Hash}
				if !isWindows() {
					topIsolated.Files["link"] = isolated.BasicFile(isolated.HashBytes(h, barData), mode, int64(len(barData)))
				} else {
					topIsolated.Files["link"] = isolated.BasicFile(isolated.HashBytes(h, winLinkData), 0666, int64(len(winLinkData)))
				}
				isolatedData := newArchiveExpectData(h, topIsolated)
				expected := map[string]map[isolated.HexDigest]string{
					namespace: {
						isolated.HashBytes(h, barData): string(barData),
						isolated.HashBytes(h, bozData): string(bozData),
						baseData.Hash:                  baseData.String,
						isolatedData.Hash:              isolatedData.String,
						secondData.Hash:                secondData.String,
					},
				}
				if isWindows() {
					// TODO(maruel): Fix symlink support on Windows.
					expected[namespace][isolated.HashBytes(h, winLinkData)] = string(winLinkData)
				}
				actual := map[string]map[isolated.HexDigest]string{}
				for n, c := range server.Contents() {
					actual[n] = map[isolated.HexDigest]string{}
					for k, v := range c {
						actual[n][isolated.HexDigest(k)] = string(v)
					}
				}
				So(actual, ShouldResemble, expected)
				So(item.Digest(), ShouldResemble, isolatedData.Hash)
				So(server.Error(), ShouldBeNil)
				digest := isolated.HashBytes(h, buf.Bytes())
				So(digest, ShouldResemble, isolatedData.Hash)
			})
		}
	})
}

func TestArchiveFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	Convey(`Fails to archive a file`, t, func() {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()

		Convey(`File missing`, func() {
			a := archiver.New(ctx, isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil), nil)

			tmpDir, err := ioutil.TempDir("", "archiver")
			So(err, ShouldBeNil)
			defer func() {
				So(os.RemoveAll(tmpDir), ShouldBeNil)
			}()

			// This will trigger an eventual Cancel().
			nonexistent := filepath.Join(tmpDir, "nonexistent")
			item1 := a.PushFile("foo", nonexistent, 0)
			So(item1.DisplayName, ShouldResemble, "foo")

			fileName := filepath.Join(tmpDir, "existent")
			So(ioutil.WriteFile(fileName, []byte("foo"), 0600), ShouldBeNil)
			item2 := a.PushFile("existent", fileName, 0)
			item1.WaitForHashed()
			item2.WaitForHashed()
			msg := "no such file or directory"
			if runtime.GOOS == "windows" {
				// Warning: this string is localized.
				msg = "The system cannot find the file specified."
			}
			fileErr := fmt.Errorf("source(foo) failed: open %s%cnonexistent: %s", tmpDir, filepath.Separator, msg)
			// TODO(maruel): https://crbug.com/969145
			So(a.Close(), ShouldResemble, context.Canceled)
			So(item1.Error(), ShouldResemble, fileErr)
			// There can be a race with item2 having time to be sent or not, but if
			// there's an error, it must be context.Canceled.
			if err := item2.Error(); err != nil {
				So(item2.Error(), ShouldResemble, context.Canceled)
			}
			So(server.Error(), ShouldBeNil)
		})

		Convey(`Server error`, func() {
			expectedErr := errors.New("expected")
			// Override the handler to fail.
			server.ServeMux = *http.NewServeMux()
			server.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
				ioutil.ReadAll(req.Body)
				req.Body.Close()
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(400)
				j := json.NewEncoder(w)
				out := map[string]string{"error": expectedErr.Error()}
				So(j.Encode(out), ShouldBeNil)
			})
			a := archiver.New(ctx, isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil), nil)

			tmpDir, err := ioutil.TempDir("", "archiver")
			So(err, ShouldBeNil)
			defer func() {
				So(os.RemoveAll(tmpDir), ShouldBeNil)
			}()

			// This will trigger an eventual Cancel().
			fileName := filepath.Join(tmpDir, "existent")
			So(ioutil.WriteFile(fileName, []byte("foo"), 0600), ShouldBeNil)
			item := a.PushFile("existent", fileName, 0)
			item.WaitForHashed()
			So(item.Error(), ShouldResemble, nil)
			// TODO(maruel): https://crbug.com/969145
			So(a.Close(), ShouldResemble, context.Canceled)
		})
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

func oneFileArchiveExpect(h crypto.Hash, file string, mode int, data []byte) *archiveExpectData {
	i := isolated.New(h)
	i.Files[file] = isolated.BasicFile(isolated.HashBytes(h, data), mode, int64(len(data)))
	return newArchiveExpectData(h, i)
}

func newArchiveExpectData(h crypto.Hash, i *isolated.Isolated) *archiveExpectData {
	encoded, _ := json.Marshal(i)
	str := string(encoded) + "\n"
	return &archiveExpectData{
		Isolated: i,
		String:   str,
		Hash:     isolated.HashBytes(h, []byte(str)),
	}
}

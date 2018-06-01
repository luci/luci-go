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
	"fmt"
	"io/ioutil"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"

	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func TestArchiverEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`An empty archiver should produce sane output.`, t, func() {
		a := New(ctx, isolatedclient.New(nil, nil, "https://localhost:1", isolatedclient.DefaultNamespace, nil, nil), nil)
		stats := a.Stats()
		So(stats.TotalHits(), ShouldResemble, 0)
		So(stats.TotalMisses(), ShouldResemble, 0)
		So(stats.TotalBytesHits(), ShouldResemble, units.Size(0))
		So(stats.TotalBytesPushed(), ShouldResemble, units.Size(0))
		So(a.Close(), ShouldBeNil)
	})
}

func TestArchiverFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`An archiver should handle file archival operations.`, t, func() {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()
		a := New(ctx, isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil), nil)

		fEmpty, err := ioutil.TempFile("", "archiver")
		So(err, ShouldBeNil)
		item1 := a.PushFile(fEmpty.Name(), fEmpty.Name(), 0)
		So(item1.DisplayName, ShouldResemble, fEmpty.Name())
		fFoo, err := ioutil.TempFile("", "archiver")
		So(err, ShouldBeNil)
		So(ioutil.WriteFile(fFoo.Name(), []byte("foo"), 0600), ShouldBeNil)
		item2 := a.PushFile(fFoo.Name(), fFoo.Name(), 0)
		// Push the same file another time. It'll get linked to the first.
		item3 := a.PushFile(fFoo.Name(), fFoo.Name(), 0)
		item1.WaitForHashed()
		item2.WaitForHashed()
		item3.WaitForHashed()
		So(a.Close(), ShouldBeNil)

		stats := a.Stats()
		So(stats.TotalHits(), ShouldResemble, 0)
		// Only 2 lookups, not 3.
		So(stats.TotalMisses(), ShouldResemble, 2)
		So(stats.TotalBytesHits(), ShouldResemble, units.Size(0))
		So(stats.TotalBytesPushed(), ShouldResemble, units.Size(3))
		expected := map[isolated.HexDigest][]byte{
			"0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33": []byte("foo"),
			"da39a3ee5e6b4b0d3255bfef95601890afd80709": {},
		}
		So(server.Contents(), ShouldResemble, expected)
		So(item1.Digest(), ShouldResemble, isolated.HexDigest("da39a3ee5e6b4b0d3255bfef95601890afd80709"))
		So(item1.Error(), ShouldBeNil)
		So(item2.Digest(), ShouldResemble, isolated.HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33"))
		So(item2.Error(), ShouldBeNil)
		So(item3.Digest(), ShouldResemble, isolated.HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33"))
		So(item3.Error(), ShouldBeNil)
		So(server.Error(), ShouldBeNil)
	})
}

func TestArchiverFileHit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`An archiver should correctly cache an archived file.`, t, func() {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()
		a := New(ctx, isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil), nil)
		server.Inject([]byte("foo"))
		item := a.Push("foo", isolatedclient.NewBytesSource([]byte("foo")), 0)
		item.WaitForHashed()
		So(item.Digest(), ShouldResemble, isolated.HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33"))
		So(a.Close(), ShouldBeNil)

		stats := a.Stats()
		So(stats.TotalHits(), ShouldResemble, 1)
		So(stats.TotalMisses(), ShouldResemble, 0)
		So(stats.TotalBytesHits(), ShouldResemble, units.Size(3))
		So(stats.TotalBytesPushed(), ShouldResemble, units.Size(0))
	})
}

func TestArchiverCancel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`A cancelled archiver should produce sane output.`, t, func() {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()
		a := New(ctx, isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil), nil)

		tmpDir, err := ioutil.TempDir("", "archiver")
		So(err, ShouldBeNil)
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Fail()
			}
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
		osErr := "no such file or directory"
		if common.IsWindows() {
			osErr = "The system cannot find the file specified."
		}
		expected := fmt.Errorf("source(foo) failed: open %s: %s", nonexistent, osErr)
		So(<-a.Channel(), ShouldResemble, expected)
		So(a.Close(), ShouldResemble, expected)
		So(server.Error(), ShouldBeNil)
	})
}

func TestArchiverPushClosed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`A closed archiver should ignore additional input.`, t, func() {
		a := New(ctx, nil, nil)
		So(a.Close(), ShouldBeNil)
		So(a.PushFile("ignored", "ignored", 0), ShouldBeNil)
	})
}

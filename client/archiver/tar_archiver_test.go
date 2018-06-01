// Copyright 2016 The LUCI Authors.
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
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"go.chromium.org/luci/common/isolated"

	. "github.com/smartystreets/goconvey/convey"
)

func TestItemBundle(t *testing.T) {
	// TODO(tandrii): instead of monkey patching global vars, refactor to use Go
	// Interfaces and allow t.parallel().
	oldOpen := osOpen
	osOpen = fakeOsOpen
	defer func() { osOpen = oldOpen }()

	Convey("Item Bundling works", t, func() {
		Convey("one bundle", func() {
			r, err := runShardItems([]int64{100, 200, 300}, 5000)
			So(err, ShouldBeNil)
			So(r, ShouldResemble, &shards{
				bundles: []int{3},
				sizes:   []int64{1024 + 1024 + 1024 + 1024},
				digests: []isolated.HexDigest{
					"003afa69f9b267a2cf267cc5ff715968cb05354b",
				},
			})
		})

		Convey("multi bundle", func() {
			r, err := runShardItems([]int64{100, 200, 2001, 300, 301, 400, 999}, 5000)
			So(err, ShouldBeNil)
			So(r, ShouldResemble, &shards{
				bundles: []int{2, 2, 3},
				sizes: []int64{
					1024 + 1024 + 1024,        // 100, 200
					1024 + 1024 + 2560,        // 2001, 300
					1024 + 1024 + 1024 + 1536, // 301, 400, 999
				},
				digests: []isolated.HexDigest{
					"49a8a345680b6f38f2f2d5e9fa300032d5dbdbe7",
					"f0599edd049997209916283326d29736c9ee469e",
					"81d9424320838c562d81fa22be81512f26eacffd",
				},
			})
			Convey("doesn't depend on order of tarred items", func() {
				r2, err := runShardItems([]int64{2001, 100, 300, 999, 200, 400, 301}, 5000)
				So(err, ShouldBeNil)
				So(r2, ShouldResemble, r)
			})
		})

		Convey("file below boundary", func() {
			r, err := runShardItems([]int64{511}, 5000)
			So(err, ShouldBeNil)
			So(r, ShouldResemble, &shards{
				bundles: []int{1},
				sizes:   []int64{2048},
				digests: []isolated.HexDigest{
					"6103d5cc5fc494f9490107d61a952c6ce1e48be6",
				},
			})
		})

		Convey("file on boundary", func() {
			r, err := runShardItems([]int64{512}, 5000)
			So(err, ShouldBeNil)
			So(r, ShouldResemble, &shards{
				bundles: []int{1},
				sizes:   []int64{2048},
				digests: []isolated.HexDigest{
					"8c63e33b6264ade5f62ab62b322de8d8ac5eeb8b",
				},
			})
		})

		Convey("all items over threshold", func() {
			r, err := runShardItems([]int64{5000, 6000, 4500}, 5000)
			So(err, ShouldBeNil)
			So(r, ShouldResemble, &shards{
				bundles: []int{1, 1, 1},
				sizes:   []int64{6144, 6656, 7680},
				digests: []isolated.HexDigest{
					"1f576aa4d44aea793c751bf03de2d513a2dd65eb",
					"2cd4cb7a965d9ca34cbb692e3885f7074f28a4a4",
					"925b04df322c98d013b470727df5644644d93e70",
				},
			})
		})
	})

	Convey("Item Bundling with Errors", t, func() {
		testItems := []*Item{
			{
				// File that fails to open.
				RelPath: "./open",
				Path:    "/err/open",
				Size:    123,
			},
			{
				// File that fails to read.
				RelPath: "./read",
				Path:    "/err/read",
				Size:    123,
			},
		}

		bundles := shardItems(testItems, 0)
		So(len(bundles), ShouldEqual, 2)
		for _, bundle := range bundles {
			if _, _, err := bundle.Digest(); err == nil {
				t.Errorf("Path %q, bundle.Digest gave nil error; want some error", bundle.items[0].Path)
			}
			rc, err := bundle.Contents()
			if err != nil {
				t.Errorf("Path %q, bundle.Contents gave error %v; want nil error", bundle.items[0].Path, err)
				continue
			}
			_, err = ioutil.ReadAll(rc)
			rc.Close()
			if err == nil {
				t.Errorf("Path %q, reading contents gave nil error, want some error", bundle.items[0].Path)
			}
		}
	})
}

func fakeOsOpen(name string) (io.ReadCloser, error) {
	var r io.Reader
	switch {
	case name == "/err/open":
		return nil, fmt.Errorf("failed to open %q", name)
	case name == "/err/read":
		r = iotest.TimeoutReader(strings.NewReader("sample-file"))
	case strings.HasPrefix(name, "/size/"):
		size, _ := strconv.Atoi(strings.TrimPrefix(name, "/size/"))
		r = strings.NewReader(strings.Repeat("x", size))
	default:
		r = strings.NewReader(fmt.Sprintf("I am the file with name %q", name))
	}
	return ioutil.NopCloser(r), nil
}

type shards struct {
	bundles []int
	sizes   []int64
	digests []isolated.HexDigest
}

func runShardItems(sizes []int64, threshold int64) (*shards, error) {
	// Construct the input items, generating path based on size.
	var items []*Item
	for _, size := range sizes {
		items = append(items, &Item{
			RelPath: fmt.Sprintf("./%d", size),
			Path:    fmt.Sprintf("/size/%d", size),
			Size:    size,
		})
	}
	r := &shards{}
	bundles := shardItems(items, threshold)
	for i, b := range bundles {
		digest, size, err := b.Digest()
		if err != nil {
			return nil, fmt.Errorf("bundle[%d] Digest failed: %s", i, err)
		}
		rc, err := b.Contents()
		if err != nil {
			return nil, fmt.Errorf("bundle[%d] Contents failed: %s", i, err)
		}
		defer rc.Close()
		tar, err := ioutil.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("bundle[%d] reading Contents failed: %s", i, err)
		}
		if got := len(tar); int64(got) != size {
			return nil, fmt.Errorf("bundle[%d] Contents size %d differs from Digest size %d", i, got, size)
		}
		r.bundles = append(r.bundles, len(b.items))
		r.sizes = append(r.sizes, size)
		r.digests = append(r.digests, digest)
	}
	return r, nil
}

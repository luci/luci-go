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

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/kr/pretty"
	"github.com/luci/luci-go/common/isolated"
)

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

func TestItemBundle(t *testing.T) {
	oldOpen := osOpen
	osOpen = fakeOsOpen
	defer func() { osOpen = oldOpen }()

	// All tests use an archive threshold of 5000 bytes.
	const threshold = 5000

	testCases := []struct {
		desc        string
		sizes       []int64
		wantBundles []int                // The number of items in each bundle.
		wantDigests []isolated.HexDigest // The hash of each bundle's tar.
		wantSize    []int                // The size of each bundle's tar.
	}{
		{
			desc:        "one bundle",
			sizes:       []int64{100, 200, 300},
			wantBundles: []int{3},
			wantSize:    []int{1024 + 1024 + 1024 + 1024},
			wantDigests: []isolated.HexDigest{
				"003afa69f9b267a2cf267cc5ff715968cb05354b",
			},
		},
		{
			desc:        "multi bundle",
			sizes:       []int64{100, 200, 300, 400, 500, 2000},
			wantBundles: []int{3, 2, 1},
			wantSize: []int{
				1024 + 1024 + 1024 + 1024,
				1024 + 1024 + 1024,
				1024 + 2560,
			},
			wantDigests: []isolated.HexDigest{
				"003afa69f9b267a2cf267cc5ff715968cb05354b",
				"7f8cedcc12dc7ddcbe0849a46628b2748f5e84aa",
				"e4a390dbfa081e564fe61368bac77bb9ef30a4e7",
			},
		},
		{
			desc:        "file below boundary",
			sizes:       []int64{511},
			wantBundles: []int{1},
			wantSize:    []int{2048},
			wantDigests: []isolated.HexDigest{
				"6103d5cc5fc494f9490107d61a952c6ce1e48be6",
			},
		},
		{
			desc:        "file on boundary",
			sizes:       []int64{512},
			wantBundles: []int{1},
			wantSize:    []int{2048},
			wantDigests: []isolated.HexDigest{
				"8c63e33b6264ade5f62ab62b322de8d8ac5eeb8b",
			},
		},
		{
			desc:        "all items over threshold",
			sizes:       []int64{5000, 6000, 4500},
			wantBundles: []int{1, 1, 1},
			wantSize:    []int{6656, 7680, 6144},
			wantDigests: []isolated.HexDigest{
				"2cd4cb7a965d9ca34cbb692e3885f7074f28a4a4",
				"925b04df322c98d013b470727df5644644d93e70",
				"1f576aa4d44aea793c751bf03de2d513a2dd65eb",
			},
		},
	}

	for _, tc := range testCases {
		// Construct the input items and expected output.
		var items []*Item
		for _, size := range tc.sizes {
			items = append(items, &Item{
				RelPath: fmt.Sprintf("./%d", size),
				Path:    fmt.Sprintf("/size/%d", size),
				Size:    size,
			})
		}
		var wantBundles []*ItemBundle
		temp := items
		for _, bSize := range tc.wantBundles {
			bundle := &ItemBundle{
				Items: temp[:bSize],
			}
			wantBundles = append(wantBundles, bundle)
			temp = temp[bSize:]
			for _, item := range bundle.Items {
				bundle.ItemSize += item.Size
			}
		}

		bundles := ShardItems(items, threshold)
		if !reflect.DeepEqual(bundles, wantBundles) {
			t.Errorf("%s:\ngot  %s\nwant %s", tc.desc, pretty.Sprint(bundles), pretty.Sprint(wantBundles))
		}

		for i, bundle := range bundles {
			digest, size, err := bundle.Digest()
			if err != nil {
				t.Errorf("%s: bundle[%d].Digest gave err %v, want nil", tc.desc, i, err)
				continue
			}
			if digest != tc.wantDigests[i] {
				t.Errorf("%s: bundle[%d].Digest() hash = %q, want %q", tc.desc, i, digest, tc.wantDigests[i])
			}
			if size != int64(tc.wantSize[i]) {
				t.Errorf("%s: bundle[%d].Digest() size = %d, want %d", tc.desc, i, size, tc.wantSize[i])
			}

			rc, err := bundle.Contents()
			if err != nil {
				t.Errorf("%s: bundle[%d].Contents gave err %v, want nil", tc.desc, i, err)
				continue
			}
			tar, err := ioutil.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Errorf("%s: reading bundle[%d].Contents gave err %v, want nil", tc.desc, i, err)
			}
			if got := len(tar); got != tc.wantSize[i] {
				t.Errorf("%s: bundle[%d].Contents had len %d, want %d", tc.desc, i, got, tc.wantSize[i])
			}
		}
	}
}

func TestItemBundle_Errors(t *testing.T) {
	oldOpen := osOpen
	osOpen = fakeOsOpen
	defer func() { osOpen = oldOpen }()

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

	bundles := ShardItems(testItems, 0)
	if len(bundles) != 2 {
		t.Errorf("len(bundles) = %d, want 2", len(bundles))
	}
	for _, bundle := range bundles {
		if _, _, err := bundle.Digest(); err == nil {
			t.Errorf("Path %q, bundle.Digest gave nil error; want some error", bundle.Items[0].Path)
		}
		rc, err := bundle.Contents()
		if err != nil {
			t.Errorf("Path %q, bundle.Contents gave error %v; want nil error", bundle.Items[0].Path, err)
			continue
		}
		_, err = ioutil.ReadAll(rc)
		rc.Close()
		if err == nil {
			t.Errorf("Path %q, reading contents gave nil error, want some error", bundle.Items[0].Path)
		}
	}
}

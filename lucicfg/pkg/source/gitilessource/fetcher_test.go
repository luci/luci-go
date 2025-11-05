// Copyright 2025 The LUCI Authors.
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

package gitilessource

import (
	"context"
	"os"
	"testing"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/data/stringset"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/lucicfg/pkg/source"
)

type fakeGitilesClient struct {
	gitilespb.GitilesClient
	tgzContent []byte
}

func (f *fakeGitilesClient) Archive(ctx context.Context, in *gitilespb.ArchiveRequest, opts ...grpc.CallOption) (*gitilespb.ArchiveResponse, error) {
	return &gitilespb.ArchiveResponse{
		Contents: f.tgzContent,
	}, nil
}

var _ gitilespb.GitilesClient = (*fakeGitilesClient)(nil)

func TestFetcher(t *testing.T) {
	tdir := t.TempDir()

	rc := &repoCache{
		client: &fakeGitilesClient{
			tgzContent: makeTestTarball(
				t,
				file("some/file", "stuff"),
				file("some/other/file", "more stuff"),
				symlink("some/symup", "../Things"),
				file("Things", "additional things"),
				symlink("symdown", "some/file"),
				symlink("path/to/symout", "../../../../stuff"),
				symlink("dir/broken_link", "../blerp"),

				// symlink chain
				symlink("chain/a", "b"),
				symlink("chain/b", "../chain/c"),
				symlink("chain/c", "./deeper/d"),
				symlink("chain/deeper/d", "../../Things"),

				// symlink loop
				symlink("loop/a", "b"),
				symlink("loop/b", "deep/../c"),
				symlink("loop/c", "d"),
				symlink("loop/d", "../loop/b"),
			),
		},
		repoRoot: tdir,
	}
	defer rc.shutdown(t.Context(), 0, 0)

	prep := func(t *testing.T) source.Fetcher {
		t.Helper()

		ret, err := rc.Fetcher(t.Context(), "", "fakeCommit", "some/prefix", func(kind source.ObjectKind, pkgRelPath string) bool {
			return pkgRelPath != "Things"
		})
		assert.NoErr(t, err, truth.LineContext())
		assert.Loosely(t, ret.(*fetcher).allowedBlobPaths, should.Match(stringset.NewFromSlice(
			"some/file",
			"some/other/file",
			"some/symup",
			"symdown",
			"path/to/symout",
			"dir/broken_link",
			"chain/a",
			"chain/b",
			"chain/c",
			"chain/deeper/d",
			"loop/a",
			"loop/b",
			"loop/c",
			"loop/d",
			// "Things" is excluded
		)), truth.LineContext())

		entries, err := os.ReadDir(tdir)
		assert.NoErr(t, err, truth.LineContext())

		assert.Loosely(t, entries, should.HaveLength(1), truth.LineContext())
		// sha256("some/prefix")@fakeCommit
		assert.That(t, entries[0].Name(), should.Equal(
			"_V8sHdcx6W_DtjQVw-q8ogEREjX9mdBjsAhnQfxjvKU@fakeCommit.zip"),
			truth.LineContext())
		return ret
	}

	t.Run(`success`, func(t *testing.T) {
		fetcher := prep(t)
		dat, err := fetcher.Read(t.Context(), "some/file")
		assert.NoErr(t, err)
		assert.That(t, string(dat), should.Equal("stuff"))
	})

	t.Run(`symlinks up tree`, func(t *testing.T) {
		fetcher := prep(t)
		dat, err := fetcher.Read(t.Context(), "some/symup")
		assert.NoErr(t, err)
		assert.That(t, string(dat), should.Equal("additional things"))
	})

	t.Run(`symlinks down tree`, func(t *testing.T) {
		fetcher := prep(t)
		dat, err := fetcher.Read(t.Context(), "symdown")
		assert.NoErr(t, err)
		assert.That(t, string(dat), should.Equal("stuff"))
	})

	t.Run(`symlinks out of bundle fail`, func(t *testing.T) {
		fetcher := prep(t)
		_, err := fetcher.Read(t.Context(), "path/to/symout")
		assert.ErrIsLike(t, err, `"../../stuff": open stuff: file does not exist`)
	})

	t.Run(`symlinks to missing file fail`, func(t *testing.T) {
		fetcher := prep(t)
		_, err := fetcher.Read(t.Context(), "dir/broken_link")
		assert.ErrIsLike(t, err, `"blerp": open some/prefix/blerp: file does not exist`)
	})

	t.Run(`follows symlink chain`, func(t *testing.T) {
		fetcher := prep(t)
		dat, err := fetcher.Read(t.Context(), "chain/a")
		assert.NoErr(t, err)
		assert.That(t, string(dat), should.Equal("additional things"))
	})

	t.Run(`symlink cycle fails`, func(t *testing.T) {
		fetcher := prep(t)
		_, err := fetcher.Read(t.Context(), "loop/a")
		assert.ErrIsLike(t, err, `symlink cycle: "loop/b"`)
	})
}

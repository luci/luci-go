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

package gitsource

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"go.chromium.org/luci/common/exec/execmock"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/lucicfg/pkg/source"
)

var catFileBreakAfterOne = execmock.Register(func(none execmock.None) (_ execmock.None, code int, err error) {
	stdin := bufio.NewReader(os.Stdin)

	// pass through the first command
	okPrompt, err := stdin.ReadString(0)
	if err != nil {
		return
	}
	realCmd := exec.Command(os.Args[0], os.Args[1:]...)
	realCmd.Stdin = strings.NewReader(okPrompt)
	realCmd.Stdout = os.Stdout
	if err = realCmd.Run(); err != nil {
		return
	}

	// wait for the next prompt then suddenly exit
	stdin.ReadString(0)
	return
})

func TestCatFileBatchLazy(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		repo := mkRepo(t, "73bd1be998e9effd633fd2fc4428f8081b231fb6:subdir/PACKAGE.star")

		// This picks an arbitrary commit/file to pull through the cache.
		kind, dat, err := repo.batchProc.catFile(
			context.Background(), "73bd1be998e9effd633fd2fc4428f8081b231fb6:subdir/PACKAGE.star")
		assert.NoErr(t, err)

		check.That(t, kind.String(), should.Equal("BlobKind"))                        // for coverage
		check.That(t, source.ObjectKind(-1).String(), should.Equal("ObjectKind(-1)")) // for coverage

		check.That(t, kind, should.Equal(source.BlobKind))
		check.Loosely(t, dat, should.HaveLength(123))
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		repo := mkRepo(t)

		_, _, err := repo.batchProc.catFile(
			context.Background(), "deadbeef82d36767c5399c936e6356782cc4:noexist")
		assert.ErrIsLike(t, err, "object is missing")
	})

	t.Run("ambiguous", func(t *testing.T) {
		t.Parallel()

		objs := []string{
			"6371c00ed96239f955ecdb11c96fcd578eff821d",
			"637172bb3ec0d2d76c7e5b2b5f21b9e5bf3aae96",
		}

		repo := mkRepo(t, objs...)

		// there are two actual objects whose hashes start with `6371`
		kind, _, err := repo.batchProc.catFile(context.Background(), objs[0])
		assert.NoErr(t, err)
		assert.That(t, kind, should.Equal(source.BlobKind))

		kind, _, err = repo.batchProc.catFile(context.Background(), objs[1])
		assert.NoErr(t, err)
		assert.That(t, kind, should.Equal(source.BlobKind))

		_, _, err = repo.batchProc.catFile(
			context.Background(), "6371")
		assert.ErrIsLike(t, err, "ambiguous")
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		obj := "6371c00ed96239f955ecdb11c96fcd578eff821d"

		repo := mkRepo(t, obj)

		// can get blob we know about
		kind, dat, err := repo.batchProc.catFile(context.Background(), obj)
		assert.NoErr(t, err)
		check.That(t, kind, should.Equal(source.BlobKind))
		check.Loosely(t, dat, should.HaveLength(79))

		// but we immediately fail to pull another blob which exists, but we
		// haven't pulled in yet.
		_, _, err = repo.batchProc.catFile(
			context.Background(), "73bd1be998e9effd633fd2fc4428f8081b231fb6")
		assert.ErrIsLike(t, err, "missing")
	})

	t.Run("canceled", func(t *testing.T) {
		t.Parallel()

		target := "73bd1be998e9effd633fd2fc4428f8081b231fb6:subdir/PACKAGE.star"

		repo := mkRepo(t, target)

		cause := fmt.Errorf("I am a banana")

		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(cause)

		_, _, err := repo.batchProc.catFile(ctx, target)
		assert.ErrIsLike(t, err, cause)

		// even though catFile set up a subprocess, it's nil again after
		// cancellation.
		assert.Loosely(t, repo.batchProc.cmd, should.BeNil)

		// we can still use repo.batchProcLazy after cancelation, it will just make
		// a new subprocess.
		kind, dat, err := repo.batchProc.catFile(context.Background(), target)
		assert.NoErr(t, err)
		assert.That(t, kind, should.Equal(source.BlobKind))
		assert.Loosely(t, dat, should.HaveLength(123))
	})

	t.Run("injected error gets shutdown", func(t *testing.T) {
		t.Parallel()

		target := "73bd1be998e9effd633fd2fc4428f8081b231fb6:subdir/PACKAGE.star"

		repo := mkRepo(t, target)

		ctx := execmock.Init(context.Background())
		errHits := catFileBreakAfterOne.WithArgs("git", "--git-dir", "...", "cat-file").Mock(ctx)

		kind, dat, err := repo.batchProc.catFile(
			ctx, "73bd1be998e9effd633fd2fc4428f8081b231fb6:subdir/PACKAGE.star")
		assert.NoErr(t, err)
		assert.That(t, kind, should.Equal(source.BlobKind))
		assert.Loosely(t, dat, should.HaveLength(123))
		assert.Loosely(t, repo.batchProc.cmd, should.NotBeNil)

		_, _, err = repo.batchProc.catFile(
			ctx, "73bd1be998e9effd633fd2fc4428f8081b231fb6:subdir/PACKAGE.star")
		assert.ErrIsLike(t, err, io.EOF)
		assert.Loosely(t, repo.batchProc.cmd, should.BeNil)

		assert.Loosely(t, errHits.Snapshot(), should.HaveLength(1))
	})
}

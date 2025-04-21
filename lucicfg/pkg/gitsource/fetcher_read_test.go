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
	"context"
	"strings"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestReadNoPrefetch(t *testing.T) {
	t.Parallel()

	repo := mkRepo(t)

	_, err := repo.Fetcher(
		context.Background(), "refs/heads/main", "e0d8ad8ebb113bc1eb10be628cd57e3a7684df4f", "subdir", nil)
	assert.ErrIsLike(t, err, "prefetch function is required")
}

func TestFetcherNoCommit(t *testing.T) {
	t.Parallel()

	repo := mkRepo(t)

	_, err := repo.Fetcher(
		context.Background(), "refs/heads/main", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "subdir", func(kind ObjectKind, pkgRelPath string) bool {
			return true
		})
	assert.ErrIsLike(t, err, ErrMissingObject)
}

func TestFetcherNoSubdir(t *testing.T) {
	t.Parallel()

	repo := mkRepo(t)

	_, err := repo.Fetcher(
		context.Background(), "refs/heads/main", "e0d8ad8ebb113bc1eb10be628cd57e3a7684df4f", "I DO NOT EXIST", func(kind ObjectKind, pkgRelPath string) bool {
			return true
		})
	assert.ErrIsLike(t, err, ErrMissingObject)
}

func TestReadPrefetch(t *testing.T) {
	t.Parallel()

	repo := mkRepo(t)

	sawNames := stringset.New(10)
	fetcher, err := repo.Fetcher(
		context.Background(), "refs/heads/main", "e0d8ad8ebb113bc1eb10be628cd57e3a7684df4f", "subdir", func(kind ObjectKind, pkgRelPath string) bool {
			sawNames.Add(pkgRelPath)
			return kind != BlobKind || strings.HasSuffix(pkgRelPath, ".star")
		})
	assert.NoErr(t, err)

	assert.That(t, sawNames, should.Match(stringset.NewFromSlice(
		"PACKAGE.star",
		"UNREFERENCED_DATA",
		"builders.json",
		"generated",
		"generated/cr-buildbucket.cfg",
		"generated/project.cfg",
		"generated/realms.cfg",
		"main.star",
	)))

	// make sure we prefetched PACKAGE.star
	_, _, err = repo.batchProc.catFile(context.Background(), "fa70b1cd267f69a7a6ea237364ae3b71907b5005")
	assert.NoErr(t, err)

	dat, err := fetcher.Read(context.Background(), "PACKAGE.star")
	assert.NoErr(t, err)
	assert.Loosely(t, dat, should.HaveLength(123))
}

func TestReadPrefetchMissingBlob(t *testing.T) {
	t.Parallel()

	repo := mkRepo(t)

	fetcher, err := repo.Fetcher(
		context.Background(), "refs/heads/main", "e0d8ad8ebb113bc1eb10be628cd57e3a7684df4f", "subdir", func(kind ObjectKind, pkgRelPath string) bool {
			return kind != BlobKind || strings.HasSuffix(pkgRelPath, ".star")
		})
	assert.NoErr(t, err)

	// make sure we prefetched PACKAGE.star
	_, _, err = repo.batchProc.catFile(context.Background(), "fa70b1cd267f69a7a6ea237364ae3b71907b5005")
	assert.NoErr(t, err)

	_, err = fetcher.Read(context.Background(), "Does Not Exist")
	assert.ErrIsLike(t, err, ErrObjectNotPrefetched)
}

func TestReadPrefetchMissingCommit(t *testing.T) {
	t.Parallel()

	repo := mkRepo(t)

	_, err := repo.Fetcher(
		context.Background(), "refs/heads/main", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "subdir", func(kind ObjectKind, pkgRelPath string) bool {
			return kind != BlobKind || strings.HasSuffix(pkgRelPath, ".star")
		})
	assert.ErrIsLike(t, err, ErrMissingObject)
}

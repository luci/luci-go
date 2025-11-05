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

// Package source contains interfaces which cover lucicfg package data sources.
//
// Implementations include:
//   - ../gitsource
//   - ../gitilessource
package source

import "context"

// Cache is a root object which allows management of multiple RepoCaches.
type Cache interface {
	// ForRepo returns a repo-specific cache object which allows you to create new
	// Fetchers which implement [Fetcher].
	//
	// The state for the RepoCache will be a subdirectory of the Cache's cacheRoot,
	// using Base64URLSafeRaw(SHA256(url)) as the subdirectory name. The
	// responsibility of normalizing repo URLs falls on the caller (e.g. if
	// `.../blah` and `.../blah.git` actually are the same repo, this function will
	// return two separate caches for them).
	//
	// Calling ForRepo multiple times with the same `url` will return an identical
	// RepoCache.
	ForRepo(ctx context.Context, url string) (RepoCache, error)

	// Releases any process resources associated with this Cache.
	Shutdown(context.Context)
}

// RepoCache caches interactions with a single remote repo.
type RepoCache interface {
	// Fetcher returns a new Fetcher pinned to the given ref, commit, and pkgRoot in
	// this repo.
	//
	// `ref` must be in the form `refs/*`, and must contain `commit`. If a ref is
	// provided which does not contain `commit`, this returns ErrMissingObject.
	//
	// `commit` must be a full git commit id as hex.
	//
	// `pkgRoot` is the subdirectory within this commit that arguments to Read are
	// relative to. If this is "", it means that the pkgRelPath's are relative to
	// the root of the commit.
	//
	// `prefetch` MUST be provided, it will be called once for each entry in the git
	// tree corresponding to `commit` which is contained within `pkgRoot`. If this
	// returns `true`, then the indicated object will be added to a list of prefetch
	// objects and fetched before this function returns. If this returns `false` for
	// a TreeKind entry, traversing that entire subtree will be skipped.
	//
	// The returned Fetcher will ONLY be allowed to read data prefetched by the
	// `prefetch` function.
	//
	// Returns ErrMissingObject if `commit` or `commit:pkgRoot` are unknown.
	Fetcher(ctx context.Context, ref, commit, pkgRoot string, prefetch func(kind ObjectKind, pkgRelPath string) bool) (Fetcher, error)

	// PickMostRecent returns the most recent commit.
	//
	// All commits must be reachable from `ref`.
	PickMostRecent(ctx context.Context, ref string, commits []string) (string, error)
}

// The smallest possible interface for interacting with the data of a remote
// repo.
//
// This will always internally resolve symlinks in the remote repo.
type Fetcher interface {
	// Returns the content of the file relative to this fetcher's pkgRoot.
	//
	// `pkgRelPath` must have been selected during `prefetch`.
	Read(ctx context.Context, pkgRelPath string) ([]byte, error)
}

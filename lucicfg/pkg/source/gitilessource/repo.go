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
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"

	"go.chromium.org/luci/lucicfg/pkg/source"
)

type repoCache struct {
	// Already wired w/ correct auth to the correct host.
	client gitilespb.GitilesClient

	// e.g. infra/infra
	project string

	// path/to/<cache.repoRoot>/<sha256(remoteUrl)>
	repoRoot string

	// protects openZips
	mu sync.RWMutex

	// We open zips once per target, even with different prefetch filters.
	//
	// Append-only until shutdown()
	openZips map[string]*zip.ReadCloser
}

func (r *repoCache) Fetcher(ctx context.Context, ref, commit, pkgRoot string, prefetch func(kind source.ObjectKind, pkgRelPath string) bool) (source.Fetcher, error) {
	if prefetch == nil {
		return nil, errors.New("gitilessource.RepoCache.Fetcher: prefetch function is required (was nil)")
	}
	pkgRoot, err := source.NormalizePkgRoot(pkgRoot)
	if err != nil {
		return nil, err
	}

	targetZip := filepath.Join(
		r.repoRoot, fmt.Sprintf("%s@%s.zip", source.HashString(pkgRoot), commit))
	// Try touching the file; if it fails, fetch it into place.
	if err = os.Chtimes(targetZip, time.Time{}, time.Now()); err != nil {
		// doesn't exist or some other error; wipe and repopulate
		os.Remove(targetZip)

		rsp, err := r.client.Archive(ctx, &gitilespb.ArchiveRequest{
			Project: r.project,
			Ref:     commit,
			Format:  gitilespb.ArchiveRequest_GZIP,
			Path:    pkgRoot,
		})
		if err != nil {
			return nil, errors.Fmt(
				"gitilessource.RepoCache.Fetcher: fetching project=%q commit=%q path=%q: %w",
				r.project, commit, pkgRoot, err)
		}

		ofile, err := os.CreateTemp(filepath.Dir(targetZip), filepath.Base(targetZip))
		if err != nil {
			return nil, errors.Fmt("gitilessource.RepoCache.Fetcher: creating output tempfile in %q: %w", filepath.Dir(targetZip), err)
		}
		finishedExtract := false
		defer func() {
			if !finishedExtract {
				ofile.Close()
				os.Remove(ofile.Name())
			}
		}()

		if err := tgzToZip(rsp.Contents, pkgRoot, ofile); err != nil {
			return nil, errors.Fmt(
				"gitilessource.RepoCache.Fetcher: extracting tarball project=%q commit=%q path=%q: %w",
				r.project, commit, pkgRoot, err)
		}

		// Must close the file for Windows
		if err := ofile.Close(); err != nil {
			return nil, errors.Fmt(
				"gitilessource.RepoCache.Fetcher: closing tarball project=%q commit=%q path=%q: %w",
				r.project, commit, pkgRoot, err)
		}

		if err := os.Rename(ofile.Name(), targetZip); err != nil {
			if !os.IsExist(err) {
				return nil, errors.Fmt("gitilessource.RepoCache.Fetcher: unable to rename %q -> %q: %w", ofile.Name(), targetZip, err)
			}
		}

		// assume if the target exists it's what we want.
		finishedExtract = true
	}

	zfile, err := r.openZip(targetZip)
	if err != nil {
		return nil, errors.Fmt(
			"gitilessource.RepoCache.Fetcher: opening cached zip %q: %w",
			targetZip, err)
	}

	includedPaths := stringset.New(50)

	// There are more efficient ways to track skipDirs for large numbers of
	// skipped directories, but since we expect there to be a small number here
	// (<5), this is expected to be the simple and efficient approach.
	var skipDirs []string

	pkgRootSlash := pkgRoot
	if len(pkgRootSlash) > 0 {
		pkgRootSlash += "/"
	}
	for _, entry := range zfile.File {
		if !strings.HasPrefix(entry.Name, pkgRootSlash) {
			continue
		}
		for _, skipDir := range skipDirs {
			if strings.HasPrefix(skipDir, entry.Name) {
				continue
			}
		}

		var kind source.ObjectKind
		if entry.Mode().IsDir() {
			kind = source.TreeKind
		} else if entry.Mode()&fs.ModeSymlink != 0 {
			kind = source.SymlinkKind
		} else {
			kind = source.BlobKind
		}

		relPath := strings.TrimPrefix(entry.Name, pkgRootSlash)
		includeInFetcher := prefetch(kind, relPath)
		if kind == source.TreeKind {
			if !includeInFetcher {
				skipDirs = append(skipDirs, entry.Name)
			}
		} else if includeInFetcher {
			includedPaths.Add(relPath)
		}
	}

	return &fetcher{zfile, pkgRoot, includedPaths}, nil
}

// this was copied from "archive/zip" reader.go:split()
func zipSplit(name string) (dir, elem string, isDir bool) {
	name, isDir = strings.CutSuffix(name, "/")
	i := strings.LastIndexByte(name, '/')
	if i < 0 {
		return ".", name, isDir
	}
	return name[:i], name[i+1:], isDir
}

// this was copied from "archive/zip" reader.go:fileEntryCompare()
func zipFileNameCompare(x, y string) int {
	xdir, xelem, _ := zipSplit(x)
	ydir, yelem, _ := zipSplit(y)
	if xdir != ydir {
		return strings.Compare(xdir, ydir)
	}
	return strings.Compare(xelem, yelem)
}

// Returns the (possibly cached) open of the zipfile at `target`.
//
// This sorts the .File slice prior to returning (to enable prefetch to just
// scan the sorted list).
func (r *repoCache) openZip(target string) (*zip.ReadCloser, error) {
	r.mu.RLock()
	ret := r.openZips[target]
	r.mu.RUnlock()

	if ret != nil {
		return ret, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ret = r.openZips[target]
	if ret != nil {
		return ret, nil
	}

	ret, err := zip.OpenReader(target)
	if err != nil {
		return nil, errors.Fmt(
			"gitilessource.RepoCache.Fetcher: opening cache zip: %w", err)
	}

	slices.SortFunc(ret.File, func(x, y *zip.File) int {
		return zipFileNameCompare(x.Name, y.Name)
	})

	if r.openZips == nil {
		r.openZips = map[string]*zip.ReadCloser{}
	}

	r.openZips[target] = ret
	return ret, nil
}

// dirsOf returns all parent directories of `prefix`.
//
// Example, if prefix is "some/thing/blah", this returns:
//   - "some/"
//   - "some/thing/"
func dirsOf(prefix string) []string {
	if prefix == "" {
		return nil
	}
	var ret []string
	for dir := path.Dir(prefix); dir != "."; dir = path.Dir(dir) {
		ret = append(ret, dir+"/")
	}
	slices.Reverse(ret)
	return ret
}

func tgzToZip(tarGzContent []byte, pkgRoot string, ofile io.Writer) error {
	zout := zip.NewWriter(ofile)

	// start by adding directory entries for pkgRoot
	//
	// The "/a" is to include pkgRoot itself.
	for _, dir := range dirsOf(pkgRoot + "/a") {
		_, err := zout.Create(dir)
		if err != nil {
			return errors.Fmt("creating directory %q: %w", dir, err)
		}
	}

	// Extract the tarball to the zip.
	gzReader, err := gzip.NewReader(bytes.NewReader(tarGzContent))
	if err != nil {
		return errors.Fmt("opening gzip Reader: %w", err)
	}
	for tarReader := tar.NewReader(gzReader); ; {
		// note; lucicfg has tarinsecurepath=0 set.
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return errors.Fmt("paging tarball: %w", err)
		}

		targetFile := path.Join(pkgRoot, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeReg:
			of, err := zout.Create(targetFile)
			if err != nil {
				return errors.Fmt("creating file %q: %w", hdr.Name, err)
			}
			if _, err := io.Copy(of, tarReader); err != nil {
				return errors.Fmt("writing file %q: %w", hdr.Name, err)
			}

		case tar.TypeDir:
			// zip uses trailing "/" to indicate a directory; `hdr.Name` would end
			// with "/", however this was stripped when joining with pkgRoot above.
			_, err := zout.Create(targetFile + "/")
			if err != nil {
				return errors.Fmt("creating directory %q: %w", hdr.Name, err)
			}

		case tar.TypeSymlink:
			zhdr := &zip.FileHeader{
				Name:   targetFile,
				Method: zip.Store,
			}
			zhdr.SetMode(fs.ModeSymlink)
			of, err := zout.CreateHeader(zhdr)
			if err != nil {
				return errors.Fmt("creating symlink %q: %w", hdr.Name, err)
			}
			if _, err := io.WriteString(of, hdr.Linkname); err != nil {
				return errors.Fmt("writing symlink %q: %w", hdr.Name, err)
			}

		default:
			return errors.Fmt("unknown tarball entry type %q: %q", hdr.Typeflag, hdr.Name)
		}
	}
	if err := zout.Close(); err != nil {
		return errors.Fmt("closing zipfile: %s", err)
	}
	return nil
}

func (r *repoCache) PickMostRecent(ctx context.Context, ref string, commits []string) (string, error) {
	commitSet := stringset.NewFromSlice(commits...)

	req := &gitilespb.LogRequest{
		Project:    r.project,
		Committish: ref,
	}

	for commit, err := range gitiles.IterLog(ctx, r.client, req, gitiles.NoLimit) {
		if err != nil {
			return "", err
		}
		if commitSet.Has(commit.Id) {
			return commit.Id, nil
		}
	}

	return "", errors.Fmt("unable to find any matching commits in %q@%q", r.project, ref)
}

func (r *repoCache) shutdown(ctx context.Context, entryTTL time.Duration, staleRatio float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ozip := range r.openZips {
		ozip.Close()
	}
	r.openZips = nil

	entries, err := os.ReadDir(r.repoRoot)
	if err != nil {
		logging.Infof(ctx, "gitilessource.repoCache.shutdown: ReadDir(%q): %s", r.repoRoot, err)
		return
	}

	type candidate struct {
		entry fs.DirEntry
		info  fs.FileInfo
	}

	numFresh := 0
	pruneCandidates := make([]*candidate, 0, len(entries))

	freshThreshold := time.Now().Add(-entryTTL)

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			logging.Infof(ctx, "gitilessource.repoCache.shutdown: entry(%v).Info(): %s", entry, err)
			continue
		}
		if info.ModTime().Before(freshThreshold) {
			pruneCandidates = append(pruneCandidates, &candidate{entry, info})
		} else {
			numFresh += 1
		}
	}

	maxStale := int(math.Ceil(float64(numFresh) * staleRatio))
	if len(pruneCandidates) <= maxStale {
		return
	}

	// Reverse sort by time
	slices.SortFunc(pruneCandidates, func(a, b *candidate) int {
		return a.info.ModTime().Compare(b.info.ModTime())
	})

	// prune everything past maxStale
	for _, entry := range entries[maxStale:] {
		fullPath := filepath.Join(r.repoRoot, entry.Name())
		logging.Debugf(ctx, "gitilessource.repoCache.shutdown: pruning %q", fullPath)
		if err := os.Remove(fullPath); err != nil {
			logging.Infof(ctx, "gitilessource.repoCache.shutdown: failed to prune cache entry %q: %s", err)
		}
	}
}

func newRepoCache(client gitilespb.GitilesClient, project string, repoRoot string) (*repoCache, error) {
	if err := os.MkdirAll(repoRoot, 0777); err != nil {
		return nil, fmt.Errorf("making root dir: %w", err)
	}
	return &repoCache{
		client:   client,
		project:  project,
		repoRoot: repoRoot,
	}, nil
}

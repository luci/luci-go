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

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	humanize "github.com/dustin/go-humanize"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/isolated"
)

// TarringArchiver archives the files specified by an isolate file to the server,
// Small files are combining into tar archives before uploading.
type TarringArchiver struct {
	checker  Checker
	uploader Uploader
}

// NewTarringArchiver constructs a TarringArchiver.
func NewTarringArchiver(checker Checker, uploader Uploader) *TarringArchiver {
	return &TarringArchiver{checker: checker, uploader: uploader}
}

// Archive uploads a single isolate.
func (ta *TarringArchiver) Archive(deps []string, rootDir string, isol *isolated.Isolated, blacklist []string, isolated string) (IsolatedSummary, error) {
	parts, err := partitionDeps(deps, rootDir, blacklist)
	if err != nil {
		return IsolatedSummary{}, fmt.Errorf("partitioning deps: %v", err)
	}
	log.Printf("Expanded to the following items to be isolated:\n%s", parts)

	tracker := NewUploadTracker(ta.checker, ta.uploader, isol)
	if err := tracker.UploadDeps(parts); err != nil {
		return IsolatedSummary{}, err
	}
	return tracker.Finalize(isolated)
}

// Item represents a file or symlink referenced by an isolate file.
type Item struct {
	Path    string
	RelPath string
	Size    int64
	Mode    os.FileMode

	Digest isolated.HexDigest
}

// itemGroup is a list of Items, plus a count of the aggregate size.
type itemGroup struct {
	items     []*Item
	totalSize int64
}

func (ig *itemGroup) AddItem(item *Item) {
	ig.items = append(ig.items, item)
	ig.totalSize += item.Size
}

// partitioningWalker contains the state necessary to partition isolate deps by handling multiple os.WalkFunc invocations.
type partitioningWalker struct {
	// fsView must be initialized before walkFn is called.
	fsView common.FilesystemView

	parts partitionedDeps
}

// partitionedDeps contains a list of items to be archived, partitioned into symlinks and files categorized by size.
type partitionedDeps struct {
	links          itemGroup
	filesToArchive itemGroup
	indivFiles     itemGroup
}

func (parts partitionedDeps) String() string {
	str := fmt.Sprintf("  %d symlinks\n", len(parts.links.items))
	str += fmt.Sprintf("  %d individual files (total size: %s)\n", len(parts.indivFiles.items), humanize.Bytes(uint64(parts.indivFiles.totalSize)))
	str += fmt.Sprintf("  %d files in archives (total size %s)", len(parts.filesToArchive.items), humanize.Bytes(uint64(parts.filesToArchive.totalSize)))
	return str
}

// walkFn implements filepath.WalkFunc, for use traversing a directory hierarchy to be isolated.
// It accumulates files in pw.parts, partitioned into symlinks and files categorized by size.
func (pw *partitioningWalker) walkFn(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	relPath, err := pw.fsView.RelativePath(path)
	if err != nil {
		return err
	}

	if relPath == "" { // empty string indicates skip.
		return common.WalkFuncSkipFile(info)
	}

	if info.IsDir() {
		return nil
	}

	item := &Item{
		Path:    path,
		RelPath: relPath,
		Mode:    info.Mode(),
		Size:    info.Size(),
	}

	switch {
	case item.Mode&os.ModeSymlink == os.ModeSymlink:
		pw.parts.links.AddItem(item)
	case item.Size < archiveThreshold:
		pw.parts.filesToArchive.AddItem(item)
	default:
		pw.parts.indivFiles.AddItem(item)
	}
	return nil
}

// partitionDeps walks each of the deps, partioning the results into symlinks and files categorized by size.
func partitionDeps(deps []string, rootDir string, blacklist []string) (partitionedDeps, error) {
	fsView, err := common.NewFilesystemView(rootDir, blacklist)
	if err != nil {
		return partitionedDeps{}, err
	}

	walker := partitioningWalker{fsView: fsView}
	for _, dep := range deps {
		// Try to walk dep. If dep is a file (or symlink), the inner function is called exactly once.
		if err := filepath.Walk(filepath.Clean(dep), walker.walkFn); err != nil {
			return partitionedDeps{}, err
		}
	}
	return walker.parts, nil
}

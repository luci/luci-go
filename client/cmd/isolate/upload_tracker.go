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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	humanize "github.com/dustin/go-humanize"
	"github.com/luci/luci-go/common/isolated"
	"github.com/luci/luci-go/common/isolatedclient"
)

// UploadTracker uploads and keeps track of files.
type UploadTracker struct {
	checker  *Checker
	uploader *Uploader
	isol     *isolated.Isolated
}

// NewUploadTracker constructs an UploadTracker.  It tracks uploaded files in isol.Files.
func NewUploadTracker(checker *Checker, uploader *Uploader, isol *isolated.Isolated) *UploadTracker {
	isol.Files = make(map[string]isolated.File)
	return &UploadTracker{
		checker:  checker,
		uploader: uploader,
		isol:     isol,
	}
}

// UploadDeps uploads all of the items in parts.
func (ut *UploadTracker) UploadDeps(parts partitionedDeps) error {
	if err := ut.populateSymlinks(parts.links.items); err != nil {
		return err
	}

	if err := ut.tarAndUploadFiles(parts.filesToArchive.items); err != nil {
		return err
	}

	if err := ut.uploadFiles(parts.indivFiles.items); err != nil {
		return err
	}
	return nil
}

// Files returns the files which have been uploaded.
// Note: files may not have completed uploading until the tracker's Checker and
// Uploader have been closed.
func (ut *UploadTracker) Files() map[string]isolated.File {
	return ut.isol.Files
}

// populateSymlinks adds an isolated.File to files for each provided symlink
func (ut *UploadTracker) populateSymlinks(symlinks []*Item) error {
	for _, item := range symlinks {
		l, err := os.Readlink(item.Path)
		if err != nil {
			return fmt.Errorf("unable to resolve symlink for %q: %v", item.Path, err)
		}
		ut.isol.Files[item.RelPath] = isolated.SymLink(l)
	}
	return nil
}

// tarAndUploadFiles creates bundles of files, uploads them, and adds each bundle to files.
func (ut *UploadTracker) tarAndUploadFiles(smallFiles []*Item) error {
	bundles := ShardItems(smallFiles, archiveMaxSize)
	log.Printf("\t%d TAR archives to be isolated", len(bundles))

	for _, bundle := range bundles {
		bundle := bundle
		digest, tarSize, err := bundle.Digest()
		if err != nil {
			return err
		}

		log.Printf("Created tar archive %q (%s)", digest, humanize.Bytes(uint64(tarSize)))
		log.Printf("\tcontains %d files (total %s)", len(bundle.Items), humanize.Bytes(uint64(bundle.ItemSize)))
		// Mint an item for this tar.
		item := &Item{
			Path:    fmt.Sprintf(".%s.tar", digest),
			RelPath: fmt.Sprintf(".%s.tar", digest),
			Size:    tarSize,
			Mode:    0644, // Read
			Digest:  digest,
		}
		ut.isol.Files[item.RelPath] = isolated.TarFile(item.Digest, int(item.Mode), item.Size)

		ut.checker.AddItem(item, false, func(item *Item, ps *isolatedclient.PushState) {
			if ps == nil {
				return
			}
			log.Printf("QUEUED %q for upload", item.RelPath)
			ut.uploader.Upload(item.RelPath, bundle.Contents, ps, func() {
				log.Printf("UPLOADED %q", item.RelPath)
			})
		})
	}
	return nil
}

// uploadFiles uploads each file and adds it to files.
func (ut *UploadTracker) uploadFiles(files []*Item) error {
	// Handle the large individually-uploaded files.
	for _, item := range files {
		d, err := hashFile(item.Path)
		if err != nil {
			return err
		}
		item.Digest = d
		ut.isol.Files[item.RelPath] = isolated.BasicFile(item.Digest, int(item.Mode), item.Size)
		ut.checker.AddItem(item, false, func(item *Item, ps *isolatedclient.PushState) {
			if ps == nil {
				return
			}
			log.Printf("QUEUED %q for upload", item.RelPath)
			ut.uploader.UploadFile(item, ps, func() {
				log.Printf("UPLOADED %q", item.RelPath)
			})
		})
	}
	return nil
}

// Finalize creates and uploads the isolate JSON at the isolatePath, and closes the checker and uploader.
// It returns the isolate digest.
// If dumpJSONPath is non-empty, the digest is also written to that path as
// JSON (in the same format as batch_archive).
// Finalize should only be called after UploadDeps.
func (ut *UploadTracker) Finalize(isolatedPath, dumpJSONPath string) (isolated.HexDigest, error) {
	// Marshal the isolated file into JSON, and create an Item to describe it.
	isolJSON, err := json.Marshal(ut.isol)
	if err != nil {
		return "", err
	}
	isolItem := &Item{
		Path:    isolatedPath,
		RelPath: filepath.Base(isolatedPath),
		Digest:  isolated.HashBytes(isolJSON),
		Size:    int64(len(isolJSON)),
	}

	// Check and upload isolate JSON.
	ut.checker.AddItem(isolItem, true, func(item *Item, ps *isolatedclient.PushState) {
		if ps == nil {
			return
		}
		log.Printf("QUEUED %q for upload", item.RelPath)
		ut.uploader.UploadBytes(item.RelPath, isolJSON, ps, func() {
			log.Printf("UPLOADED %q", item.RelPath)
		})
	})

	// Make sure that all pending items have been checked.
	if err := ut.checker.Close(); err != nil {
		return "", err
	}

	// Make sure that all the uploads have completed successfully.
	if err := ut.uploader.Close(); err != nil {
		return "", err
	}

	// Write the isolated file, and emit its digest to stdout.
	if err := ioutil.WriteFile(isolatedPath, isolJSON, 0644); err != nil {
		return "", err
	}

	fmt.Printf("%s\t%s\n", isolItem.Digest, filepath.Base(isolatedPath))

	if err := dumpJSON(isolatedPath, dumpJSONPath, isolItem); err != nil {
		return "", err
	}

	return isolItem.Digest, nil
}

func dumpJSON(isolatedPath, dumpJSONPath string, isolItem *Item) error {
	if dumpJSONPath == "" {
		return nil
	}
	// The name is the base name of the isolated file, extension stripped.
	name := filepath.Base(isolatedPath)
	if i := strings.LastIndex(name, "."); i != -1 {
		name = name[:i]
	}
	j, err := json.Marshal(map[string]isolated.HexDigest{
		name: isolItem.Digest,
	})
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(dumpJSONPath, j, 0644); err != nil {
		return err
	}
	return nil
}

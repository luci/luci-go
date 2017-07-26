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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolate"
	"github.com/luci/luci-go/common/auth"
	logpb "github.com/luci/luci-go/common/eventlog/proto"
	"github.com/luci/luci-go/common/isolated"
	"github.com/luci/luci-go/common/isolatedclient"
)

const (
	// archiveThreshold is the size (in bytes) used to determine whether to add
	// files to a tar archive before uploading. Files smaller than this size will
	// be combined into archives before being uploaded to the server.
	archiveThreshold = 100e3 // 100kB

	// archiveMaxSize is the maximum size of the created archives.
	archiveMaxSize = 10e6

	// infraFailExit is the exit code used when the exparchive fails due to
	// infrastructure errors (for example, failed server requests).
	infraFailExit = 2
)

func cmdExpArchive(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "exparchive <options>",
		ShortDesc: "EXPERIMENTAL parses a .isolate file to create a .isolated file, and uploads it and all referenced files to an isolate server",
		LongDesc:  "All the files listed in the .isolated file are put in the isolate server cache. Small files are combined together in a tar archive before uploading.",
		CommandRun: func() subcommands.CommandRun {
			c := &expArchiveRun{}
			c.commonServerFlags.Init(defaultAuthOpts)
			c.isolateFlags.Init(&c.Flags)
			c.loggingFlags.Init(&c.Flags)
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "",
				"Write isolated digests of archived trees to this file as JSON")
			return c
		},
	}
}

// expArchiveRun contains the logic for the experimental archive subcommand.
// It implements subcommand.CommandRun
type expArchiveRun struct {
	commonServerFlags // Provides the GetFlags method.
	isolateFlags      isolateFlags
	loggingFlags      loggingFlags
	dumpJSON          string
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

// main contains the core logic for experimental archive.
func (c *expArchiveRun) main() error {
	// TODO(djd): This func is long and has a lot of internal complexity (like,
	// such as, archiveCallback). Refactor.

	start := time.Now()
	archiveOpts := &c.isolateFlags.ArchiveOptions
	// Parse the incoming isolate file.
	deps, rootDir, isol, err := isolate.ProcessIsolate(archiveOpts)
	if err != nil {
		return fmt.Errorf("failed to process isolate: %v", err)
	}
	log.Printf("Isolate referenced %d deps", len(deps))

	// Set up a background context which is cancelled when this function returns.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create the isolated client which connects to the isolate server.
	authCl, err := c.createAuthClient()
	if err != nil {
		return err
	}
	client := isolatedclient.New(nil, authCl, c.isolatedFlags.ServerURL, c.isolatedFlags.Namespace, nil, nil)

	// Set up a checker and uploader. We limit the uploader to one concurrent
	// upload, since the uploads are all coming from disk (with the exception of
	// the isolated JSON itself) and we only want a single goroutine reading from
	// disk at once.
	checker := NewChecker(ctx, client)
	uploader := NewUploader(ctx, client, 1)

	parts, err := partitionDeps(deps, rootDir, c.isolateFlags.ArchiveOptions.Blacklist)
	if err != nil {
		return fmt.Errorf("partitioning deps: %v", err)
	}

	numFiles := len(parts.filesToArchive.items) + len(parts.indivFiles.items)
	filesSize := uint64(parts.filesToArchive.totalSize + parts.indivFiles.totalSize)
	log.Printf("Isolate expanded to %d files (total size %s) and %d symlinks", numFiles, humanize.Bytes(filesSize), len(parts.links.items))
	log.Printf("\t%d files (%s) to be isolated individually", len(parts.indivFiles.items), humanize.Bytes(uint64(parts.indivFiles.totalSize)))
	log.Printf("\t%d files (%s) to be isolated in archives", len(parts.filesToArchive.items), humanize.Bytes(uint64(parts.filesToArchive.totalSize)))

	tracker := NewUploadTracker(checker, uploader)
	if err := tracker.UploadDeps(parts); err != nil {
		return err
	}

	isol.Files = tracker.Files()

	// Marshal the isolated file into JSON, and create an Item to describe it.
	var isolJSON []byte
	isolJSON, err = json.Marshal(isol)
	if err != nil {
		return err
	}
	isolItem := &Item{
		Path:    archiveOpts.Isolated,
		RelPath: filepath.Base(archiveOpts.Isolated),
		Digest:  isolated.HashBytes(isolJSON),
		Size:    int64(len(isolJSON)),
	}

	// Check and upload isolate JSON.
	checker.AddItem(isolItem, true, func(item *Item, ps *isolatedclient.PushState) {
		if ps == nil {
			return
		}
		log.Printf("QUEUED %q for upload", item.RelPath)
		uploader.UploadBytes(item.RelPath, isolJSON, ps, func() {
			log.Printf("UPLOADED %q", item.RelPath)
		})
	})

	// Make sure that all pending items have been checked.
	if err := checker.Close(); err != nil {
		return err
	}

	// Make sure that all the uploads have completed successfully.
	if err := uploader.Close(); err != nil {
		return err
	}

	// Write the isolated file, and emit its digest to stdout.
	if err := ioutil.WriteFile(archiveOpts.Isolated, isolJSON, 0644); err != nil {
		return err
	}
	fmt.Printf("%s\t%s\n", isolItem.Digest, filepath.Base(archiveOpts.Isolated))

	// Optionally, write the digest of the isolated file as JSON (in the same
	// format as batch_archive).
	if c.dumpJSON != "" {
		// The name is the base name of the isolated file, extension stripped.
		name := filepath.Base(archiveOpts.Isolated)
		if i := strings.LastIndex(name, "."); i != -1 {
			name = name[:i]
		}
		j, err := json.Marshal(map[string]isolated.HexDigest{
			name: isolItem.Digest,
		})
		if err != nil {
			return err
		}
		if err := ioutil.WriteFile(c.dumpJSON, j, 0644); err != nil {
			return err
		}
	}

	end := time.Now()

	archiveDetails := &logpb.IsolateClientEvent_ArchiveDetails{
		HitCount:    proto.Int64(int64(checker.Hit.Count)),
		MissCount:   proto.Int64(int64(checker.Miss.Count)),
		HitBytes:    &checker.Hit.Bytes,
		MissBytes:   &checker.Miss.Bytes,
		IsolateHash: []string{string(isolItem.Digest)},
	}
	eventlogger := NewLogger(ctx, c.loggingFlags.EventlogEndpoint)
	op := logpb.IsolateClientEvent_ARCHIVE.Enum()
	if err := eventlogger.logStats(ctx, op, start, end, archiveDetails); err != nil {
		log.Printf("Failed to log to eventlog: %v", err)
	}

	return nil
}

func (c *expArchiveRun) parseFlags(args []string) error {
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := c.isolateFlags.Parse(cwd, RequireIsolateFile&RequireIsolatedFile); err != nil {
		return err
	}
	return nil
}

func (c *expArchiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	fmt.Fprintln(a.GetErr(), "WARNING: this command is experimental")
	if err := c.parseFlags(args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}

func hashFile(path string) (isolated.HexDigest, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return isolated.Hash(f)
}

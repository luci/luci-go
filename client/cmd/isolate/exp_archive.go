// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	humanize "github.com/dustin/go-humanize"
	"github.com/luci/luci-go/client/isolate"
	"github.com/luci/luci-go/common/isolated"
	"github.com/maruel/subcommands"
)

const (
	// archiveThreshold is the size (in bytes) used to determine whether to add
	// files to a tar archive before uploading. Files smaller than this size will
	// be combined into archives before being uploaded to the server.
	archiveThreshold = 100e3 // 100kB
)

var cmdExpArchive = &subcommands.Command{
	UsageLine: "exparchive <options>",
	ShortDesc: "EXPERIMENTAL parses a .isolate file to create a .isolated file, and uploads it and all referenced files to an isolate server",
	LongDesc:  "All the files listed in the .isolated file are put in the isolate server cache. Small files are combined together in a tar archive before uploading.",
	CommandRun: func() subcommands.CommandRun {
		c := &expArchiveRun{}
		c.commonServerFlags.Init()
		c.isolateFlags.Init(&c.Flags)
		return c
	},
}

// expArchiveRun contains the logic for the experimental archive subcommand.
// It implements subcommand.CommandRun
type expArchiveRun struct {
	commonServerFlags // Provides the GetFlags method.
	isolateFlags      isolateFlags
}

// Item represents a file or symlink referenced by an isolate file.
type Item struct {
	Path    string
	RelPath string
	Size    int64
	Mode    os.FileMode
}

// main contains the core logic for experimental archive.
func (c *expArchiveRun) main() error {
	archiveOpts := &c.isolateFlags.ArchiveOptions
	// Parse the incoming isolate file.
	deps, rootDir, isol, err := isolate.ProcessIsolate(archiveOpts)
	if err != nil {
		return fmt.Errorf("failed to process isolate: %v", err)
	}
	log.Printf("Isolate referenced %d deps", len(deps))

	// Walk each of the deps, partioning the results into symlinks and files categorised by size.
	var links, archiveFiles, indivFiles []*Item
	var archiveSize, indivSize int64 // Cumulative size of archived/individual files.
	for _, dep := range deps {
		// Try to walk dep. If dep is a file (or symlink), the inner function is called exactly once.
		err := filepath.Walk(filepath.Clean(dep), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			relPath, err := filepath.Rel(rootDir, path)
			if err != nil {
				return err
			}

			item := &Item{
				Path:    path,
				RelPath: relPath,
				Mode:    info.Mode(),
				Size:    info.Size(),
			}

			switch {
			case item.Mode&os.ModeSymlink == os.ModeSymlink:
				links = append(links, item)
			case item.Size < archiveThreshold:
				archiveFiles = append(archiveFiles, item)
				archiveSize += item.Size
			default:
				indivFiles = append(indivFiles, item)
				indivSize += item.Size
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	log.Printf("Isolate expanded to %d files (total size %s) and %d symlinks", len(archiveFiles)+len(indivFiles), humanize.Bytes(uint64(archiveSize+indivSize)), len(links))
	log.Printf("\t%d files (%s) to be isolated individually", len(indivFiles), humanize.Bytes(uint64(indivSize)))
	log.Printf("\t%d files (%s) to be isolated in archives", len(archiveFiles), humanize.Bytes(uint64(archiveSize)))

	// TODO(djd): actually do something with the each of links, archiveFiles and indivFiles.

	// Marshal the isolated file into JSON.
	isolJSON, err := json.Marshal(isol)
	if err != nil {
		return err
	}
	// TODO(djd): actually check/upload the isolated.

	// Write the isolated file, and emit its digest to stdout.
	if err := ioutil.WriteFile(archiveOpts.Isolated, isolJSON, 0644); err != nil {
		return err
	}
	fmt.Printf("%s\t%s\n", isolated.HashBytes(isolJSON), filepath.Base(archiveOpts.Isolated))

	return errors.New("experimental archive is not implemented")
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

func (c *expArchiveRun) Run(a subcommands.Application, args []string) int {
	fmt.Fprintln(a.GetErr(), "WARNING: this command is experimental")
	if err := c.parseFlags(args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if len(c.isolateFlags.ArchiveOptions.Blacklist) != 0 {
		fmt.Fprintf(a.GetErr(), "%s: blacklist is not supported\n", a.GetName())
		return 1
	}
	if err := c.main(); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}

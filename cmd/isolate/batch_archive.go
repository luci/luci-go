// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"

	"chromium.googlesource.com/infra/swarming/client-go/internal/common"
	"chromium.googlesource.com/infra/swarming/client-go/isolate"
	"github.com/maruel/subcommands"
)

var cmdBatchArchive = &subcommands.Command{
	UsageLine: "batcharchive file1 file2 ...",
	ShortDesc: "archives multiple isolated trees at once.",
	LongDesc: `Archives multiple isolated trees at once.

Using single command instead of multiple sequential invocations allows to cut
redundant work when isolated trees share common files (e.g. file hashes are
checked only once, their presence on the server is checked only once, and
so on).

Takes a list of paths to *.isolated.gen.json files that describe what trees to
isolate. Format of files is:
{
  "version": 1,
  "dir": <absolute path to a directory all other paths are relative to>,
  "args": [list of command line arguments for single 'archive' command]
}`,
	CommandRun: func() subcommands.CommandRun {
		c := &batchArchiveRun{}
		c.Init()
		return c
	},
}

type batchArchiveRun struct {
	authFlags
	serverURL string
	namespace string
	dumpJson  string
	blacklist string
}

func (c *batchArchiveRun) Init() {
	c.authFlags.Init()
	c.Flags.StringVar(&c.serverURL, "isolate-server", "https://isolateserver-dev.appspot.com/", "")
	c.Flags.StringVar(&c.namespace, "namespace", "testing", "")
	c.Flags.StringVar(&c.dumpJson, "dump-json", "",
		"Write isolated Digestes of archived trees to this file as JSON")
	c.Flags.StringVar(&c.blacklist, "blacklist", "",
		"List of regexp to use as blacklist filter when uploading directories")
}

func (c *batchArchiveRun) Parse(a subcommands.Application, args []string) error {
	// TODO: re-use serverURL parsing?
	if c.serverURL == "" {
		return errors.New("server must be specified")
	}
	if c.namespace == "" {
		return errors.New("namespace must be specified.")
	}
	if len(args) == 0 {
		return errors.New("at least one isolate file required")
	}
	return nil
}

type genJson struct {
	Args    []string
	Dir     string
	Version int
}

func (c *batchArchiveRun) main(a subcommands.Application, args []string) error {
	var trees []isolate.Tree
	for _, genJsonPath := range args {
		var data genJson
		if err := common.ReadJSONFile(genJsonPath, &data); err != nil {
			return err
		}
		if data.Version != isolate.ISOLATED_GEN_JSON_VERSION {
			return fmt.Errorf("Invalid version %d in %s", data.Version, genJsonPath)
		}
		if isDir, err := common.IsDirectory(data.Dir); !isDir || err != nil {
			return fmt.Errorf("Invalid dir %s in %s", data.Dir, genJsonPath)
		}
		if opts, err := parseArchiveCMD(data.Args, data.Dir); err != nil {
			return err
		} else {
			trees = append(trees, isolate.Tree{data.Dir, opts})
		}
	}
	isolatedHashes, err := isolate.IsolateAndArchive(trees, c.serverURL, c.namespace)
	if err != nil && c.dumpJson != "" {
		if isolatedHashes != nil {
			return common.WriteJSONFile(c.dumpJson, isolatedHashes)
		} else {
			return common.WriteJSONFile(c.dumpJson, make(map[string]string))
		}
	}
	return err
}

func (c *batchArchiveRun) Run(a subcommands.Application, args []string) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}

func parseArchiveCMD(args []string, cwd string) (opts isolate.ArchiveOptions, err error) {
	// TODO(tandrii): call subcommand to start passing IsolateOptions as if it was running comamnd
	i := newIsolateOptions(nil)
	opts = i.ArchiveOptions
	return
}

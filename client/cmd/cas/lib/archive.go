// Copyright 2020 The LUCI Authors.
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

package lib

import (
	"context"
	"fmt"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/chunker"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/command"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/filemetadata"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/tree"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/isolated"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/signals"
)

// CmdArchive returns an object for the `archive` subcommand.
func CmdArchive() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "archive <options>...",
		ShortDesc: "archive files to CAS",
		LongDesc: `Given a list of files and directories, creates a CAS tree and digest file for
that and uploads the tree to the CAS server.

When specifying directories and files, you must also specify a current working
directory for each file and directory. The current working directory will not be
included in the archived path. For example, to archive './usr/foo/bar' and have
it appear as 'foo/bar' in the CAS tree, specify '-files ./usr:foo/bar' or
'-files usr:foo/bar'. When the CAS tree is then downloaded, it will then appear
under 'foo/bar' in the desired directory.

Note that '.' may be omitted in general, so to upload 'foo' from the current
working directory, '-files :foo' is sufficient.`,
		CommandRun: func() subcommands.CommandRun {
			c := archiveRun{}
			c.Init()
			c.Flags.Var(&c.dirs, "dirs", "Directory(ies) to archive. Specify as <working directory>:<relative path to dir>")
			c.Flags.Var(&c.files, "files", "Individual file(s) to archive. Specify as <working directory>:<relative path to file>")
			return &c
		},
	}
}

type archiveRun struct {
	commonFlags
	dirs  isolated.ScatterGather
	files isolated.ScatterGather
}

func (c *archiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.Reason("position arguments not expected").Err()
	}
	return nil
}

// getRoot returns root directory if there is only one working directory.
func getRoot(dirs, files isolated.ScatterGather) (string, error) {
	var rel0, wd0 string
	pickedOne := false
	for rel, wd := range dirs {
		if !pickedOne {
			rel0 = rel
			wd0 = wd
			pickedOne = true
			continue
		}

		if wd0 != wd {
			return "", errors.Reason("different root (working) directory is not supported: %s:%s vs %s:%s", wd0, rel0, wd, rel).Err()
		}
	}

	for rel, wd := range files {
		if !pickedOne {
			rel0 = rel
			wd0 = wd
			pickedOne = true
			continue
		}

		if wd0 != wd {
			return "", errors.Reason("different root (working) directory is not supported: %s:%s vs %s:%s", wd0, rel0, wd, rel).Err()
		}
	}

	if !pickedOne {
		return "", errors.Reason("-dirs or -files should be specified at least once").Err()
	}

	return wd0, nil
}

// Does the archive by uploading to isolate-server, then return the archive stats and error.
func (c *archiveRun) doArchive(ctx context.Context, args []string) error {
	ctx, cancel := context.WithCancel(ctx)
	signals.HandleInterrupt(cancel)

	root, err := getRoot(c.dirs, c.files)
	if err != nil {
		return err
	}

	is := command.InputSpec{}
	for dir := range c.dirs {
		is.Inputs = append(is.Inputs, dir)
	}
	for file := range c.files {
		is.Inputs = append(is.Inputs, file)
	}

	rootDg, chunkers, _, err := tree.ComputeMerkleTree(root, &is, chunker.DefaultChunkSize, filemetadata.NewNoopCache())
	if err != nil {
		return errors.Annotate(err, "failed to call ComputeMerkleTree").Err()
	}

	client, err := c.casFlags.NewClient(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to create cas client").Err()
	}
	defer client.Close()

	if err := client.UploadIfMissing(ctx, chunkers...); err != nil {
		return errors.Annotate(err, "failed to call UploadIfMissing").Err()
	}

	return c.casFlags.WriteDigest(rootDg)
}

func (c *archiveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, c, env)

	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}

	if err := c.doArchive(ctx, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}

	return 0
}

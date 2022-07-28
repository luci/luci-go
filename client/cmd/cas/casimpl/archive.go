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

package casimpl

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/command"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/filemetadata"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/signals"
)

// CmdArchive returns an object for the `archive` subcommand.
func CmdArchive(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "archive <options>...",
		ShortDesc: "archive dirs/files to CAS",
		LongDesc: `Given a list of files and directories, creates a CAS tree and digest file for
that and uploads the tree to the CAS server.

When specifying directories and files, you must also specify a current working
directory for each file and directory. The current working directory will not be
included in the archived path. For example, to archive './usr/foo/bar' and have
it appear as 'foo/bar' in the CAS tree, specify '-paths ./usr:foo/bar' or
'-paths usr:foo/bar'. And all workind directories should be the same. When the
CAS tree is then downloaded, it will then appear under 'foo/bar' in the desired
directory.

Note that '.' may be omitted in general, so to upload 'foo' from the current
working directory, '-paths :foo' is sufficient.`,
		CommandRun: func() subcommands.CommandRun {
			c := archiveRun{}
			c.Init(authFlags)
			c.Flags.Var(&c.paths, "paths", "File(s)/Directory(ies) to archive. Specify as <working directory>:<relative path to file/dir>")
			c.Flags.StringVar(&c.dumpDigest, "dump-digest", "", "Dump uploaded CAS root digest to file in the format of '<Hash>/<Size>'")
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "", "Dump upload stats to json file.")
			c.Flags.StringVar(&c.dumpJSON, "dump-stats-json", "", "Dump upload stats to json file (deprecated).")
			return &c
		},
	}
}

type archiveRun struct {
	commonFlags
	paths      scatterGather
	dumpDigest string
	dumpJSON   string
}

func (c *archiveRun) parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.Reason("position arguments not expected").Err()
	}
	return nil
}

// getRoot returns root directory if there is only one working directory.
func getRoot(paths scatterGather) (string, error) {
	var rel0, wd0 string
	pickedOne := false
	for rel, wd := range paths {
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
		return "", errors.Reason("-paths should be specified at least once").Err()
	}

	return wd0, nil
}

// Does the archive by uploading to RBE-CAS.
func (c *archiveRun) doArchive(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer signals.HandleInterrupt(cancel)()
	ctx, err := casclient.ContextWithMetadata(ctx, "cas")
	if err != nil {
		return err
	}

	root, err := getRoot(c.paths)
	if err != nil {
		if err := writeExitResult(c.dumpJSON, IOError); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return err
	}

	is := command.InputSpec{Inputs: make([]string, 0, len(c.paths))}
	for path := range c.paths {
		is.Inputs = append(is.Inputs, path)
	}

	client, err := c.authFlags.NewRBEClient(ctx, c.casFlags.Addr, c.casFlags.Instance, false)
	if err != nil {
		if err := writeExitResult(c.dumpJSON, ClientError); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return errors.Annotate(err, "failed to create cas client").Err()
	}
	defer client.Close()

	start := time.Now()

	rootDg, entries, _, err := client.ComputeMerkleTree(root, "", "", &is, filemetadata.NewNoopCache())
	if err != nil {
		if err := writeExitResult(c.dumpJSON, IOError); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return errors.Annotate(err, "failed to call ComputeMerkleTree").Err()
	}
	logging.Infof(ctx, "ComputeMerkleTree took %s", time.Since(start))

	start = time.Now()
	uploadedDigests, moved, err := client.UploadIfMissing(ctx, entries...)
	if err != nil {
		if err := writeExitResult(c.dumpJSON, RPCError); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return errors.Annotate(err, "failed to call UploadIfMissing").Err()
	}
	logging.Infof(ctx, "UploadIfMissing took %s, moved %d bytes", time.Since(start), moved)

	if dd := c.dumpDigest; dd != "" {
		if err := ioutil.WriteFile(dd, []byte(rootDg.String()), 0600); err != nil {
			if err := writeExitResult(c.dumpJSON, IOError); err != nil {
				return errors.Annotate(err, "failed to write json file").Err()
			}
			return errors.Annotate(err, "failed to dump digest").Err()
		}
	} else {
		fmt.Printf("uploaded digest is %s\n", rootDg.String())
	}

	if dsj := c.dumpJSON; dsj != "" {
		uploaded := make([]int64, len(uploadedDigests))
		uploadedSet := make(map[digest.Digest]struct{})
		for i, d := range uploadedDigests {
			uploaded[i] = d.Size
			uploadedSet[d] = struct{}{}
		}

		notUploaded := make([]int64, 0, len(entries)-len(uploadedDigests))
		for _, e := range entries {
			if _, ok := uploadedSet[e.Digest]; !ok {
				notUploaded = append(notUploaded, e.Digest.Size)
			}
		}

		if err := writeStats(dsj, notUploaded, uploaded); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
	}

	return nil
}

func (c *archiveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, c, env)

	if err := c.parse(a, args); err != nil {
		errors.Log(ctx, err)
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		if err := writeExitResult(c.dumpJSON, ArgumentsInvalid); err != nil {
			fmt.Fprintf(a.GetErr(), "failed to write json file")
		}
		return 1
	}
	defer c.profiler.Stop()

	if err := c.doArchive(ctx); err != nil {
		errors.Log(ctx, err)
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}

	return 0
}

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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/client/isolated"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/isolatedclient"
)

func cmdArchive(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "archive <options>...",
		ShortDesc: "creates a .isolated file and uploads the tree to an isolate server",
		LongDesc: `Given a list of files and directories, creates a .isolated file and uploads the
tree to to an isolate server.

When specifying directories and files, you must also specify a current working
directory for that file or directory. The current working directory will not
be included in the archived path. For example, to isolate './usr/foo/bar' and
have it appear as 'foo/bar' in the .isolated, specify '-files ./usr:foo/bar' or
'-files usr:foo/bar'. When the .isolated is then downloaded, it will then appear
under 'foo/bar' in the desired directory.

Note that '.' may be omitted in general, so to upload 'foo' from the current
working directory, '-files :foo' is sufficient.`,
		CommandRun: func() subcommands.CommandRun {
			c := archiveRun{}
			c.commonFlags.Init(defaultAuthOpts)
			c.Flags.Var(&c.dirs, "dirs", "Directory(ies) to archive. Specify as <working directory>:<relative path to dir>")
			c.Flags.Var(&c.files, "files", "Individual file(s) to archive. Specify as <working directory>:<relative path to file>")
			c.Flags.Var(&c.blacklist, "blacklist",
				"List of regexp to use as blacklist filter when uploading directories")
			c.Flags.StringVar(&c.dumpHash, "dump-hash", "",
				"Write the composite isolated hash to a file")
			c.Flags.StringVar(&c.isolated, "isolated", "",
				"Write the composite isolated to a file")
			return &c
		},
	}
}

type archiveRun struct {
	commonFlags
	dirs      isolated.ScatterGather
	files     isolated.ScatterGather
	blacklist common.Strings
	dumpHash  string
	isolated  string
}

func (c *archiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	return nil
}

func (c *archiveRun) main(a subcommands.Application, args []string) (err error) {
	start := time.Now()
	out := os.Stdout
	ctx := common.CancelOnCtrlC(c.defaultFlags.MakeLoggingContext(os.Stderr))

	var authClient *http.Client
	authClient, err = c.createAuthClient(ctx)
	if err != nil {
		return
	}
	isolatedClient := isolatedclient.New(nil, authClient, c.isolatedFlags.ServerURL, c.isolatedFlags.Namespace, nil, nil)

	arch := archiver.New(ctx, isolatedClient, out)
	defer func() {
		// This waits for all uploads.
		if cerr := arch.Close(); err == nil {
			err = cerr
			return
		}
	}()

	opts := isolated.ArchiveOptions{
		Files:     c.files,
		Dirs:      c.dirs,
		Blacklist: []string(c.blacklist),
		Isolated:  c.isolated,
	}
	if len(c.isolated) != 0 {
		var dumpIsolated *os.File
		dumpIsolated, err = os.Create(c.isolated)
		if err != nil {
			return
		}
		// This is OK to close before arch because isolated.Archive
		// does the writing (it's not handed off elsewhere).
		defer dumpIsolated.Close()
		opts.LeakIsolated = dumpIsolated
	}
	item := isolated.Archive(ctx, arch, &opts)
	if err = item.Error(); err != nil {
		return
	}

	item.WaitForHashed()
	if len(c.dumpHash) != 0 {
		if err = ioutil.WriteFile(c.dumpHash, []byte(item.Digest()), 0644); err != nil {
			return
		}
	}
	if !c.defaultFlags.Quiet {
		duration := time.Since(start)
		stats := arch.Stats()
		fmt.Fprintf(os.Stderr, "Hits    : %5d (%s)\n", stats.TotalHits(), stats.TotalBytesHits())
		fmt.Fprintf(os.Stderr, "Misses  : %5d (%s)\n", stats.TotalMisses(), stats.TotalBytesPushed())
		fmt.Fprintf(os.Stderr, "Duration: %s\n", units.Round(duration, time.Millisecond))
	}
	return
}

func (c *archiveRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	cl, err := c.defaultFlags.StartTracing()
	if err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer cl.Close()
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	return 0
}

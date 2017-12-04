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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
)

func cmdArchive(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "archive <options>...",
		ShortDesc: "creates a .isolated file and uploads the tree to an isolate server.",
		LongDesc:  "All the files listed in the .isolated file are put in the isolate server.",
		CommandRun: func() subcommands.CommandRun {
			c := archiveRun{}
			c.commonFlags.Init(defaultAuthOpts)
			c.Flags.Var(&c.dirs, "dirs", "Directory(ies) to archive")
			c.Flags.Var(&c.files, "files", "Individual file(s) to archive")
			c.Flags.Var(&c.blacklist, "blacklist",
				"List of regexp to use as blacklist filter when uploading directories")
			c.Flags.StringVar(&c.dumpHash, "dump-hash", "",
				"Write the composite isolated hash to a file.")
			c.Flags.StringVar(&c.dumpIsolated, "dump-isolated", "",
				"Write the composite isolated to a file.")
			return &c
		},
	}
}

type archiveRun struct {
	commonFlags
	dirs         common.Strings
	files        common.Strings
	blacklist    common.Strings
	dumpHash     string
	dumpIsolated string
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

func (c *archiveRun) waitOnItems(items []*archiver.Item, names []string, cb func(int, isolated.HexDigest)) error {
	for i, item := range items {
		item.WaitForHashed()
		if err := item.Error(); err != nil {
			fmt.Printf("%s failed: %s\n", names[i], err)
			return err
		}
		digest := item.Digest()
		cb(i, digest)
		if !c.defaultFlags.Quiet {
			fmt.Printf("%s  %s\n", digest, names[i])
		}
	}
	return nil
}

func (c *archiveRun) main(a subcommands.Application, args []string) (err error) {
	start := time.Now()
	out := os.Stdout

	var authClient *http.Client
	authClient, err = c.createAuthClient()
	if err != nil {
		return
	}
	isolatedClient := isolatedclient.New(nil, authClient, c.isolatedFlags.ServerURL, c.isolatedFlags.Namespace, nil, nil)

	ctx := c.defaultFlags.MakeLoggingContext(os.Stderr)
	arch := archiver.New(ctx, isolatedClient, out)
	defer func() {
		// This waits for all uploads.
		if cerr := arch.Close(); err == nil {
			err = cerr
			return
		}
	}()

	common.CancelOnCtrlC(arch)
	composite := isolated.New()

	fItems := make([]*archiver.Item, 0, len(c.files))
	fNames := make([]string, 0, cap(fItems))
	for _, file := range c.files {
		info, err := os.Lstat(file)
		if err != nil {
			return err
		}
		mode := info.Mode()
		composite.Files[file] = isolated.BasicFile("", int(mode.Perm()), info.Size())

		fItems = append(fItems, arch.PushFile(file, file, 0))
		fNames = append(fNames, file)
	}

	dItems := make([]*archiver.Item, 0, len(c.dirs))
	dNames := make([]string, 0, cap(dItems))
	for _, d := range c.dirs {
		dItems = append(dItems, archiver.PushDirectory(arch, d, "", nil))
		dNames = append(dNames, d)
	}

	err = c.waitOnItems(fItems, fNames, func(i int, digest isolated.HexDigest) {
		f := composite.Files[fNames[i]]
		f.Digest = digest
		composite.Files[fNames[i]] = f
	})
	if err != nil {
		return
	}

	err = c.waitOnItems(dItems, dNames, func(_ int, digest isolated.HexDigest) {
		composite.Includes = append(composite.Includes, digest)
	})
	if err != nil {
		return
	}

	var rawComposite bytes.Buffer
	if err = json.NewEncoder(&rawComposite).Encode(composite); err != nil {
		return
	}

	var compositeName string
	if len(c.dumpIsolated) != 0 {
		compositeName = c.dumpIsolated
	} else {
		compositeName = "data.isolated"
	}
	compositeItem := arch.Push(compositeName, isolatedclient.NewBytesSource(rawComposite.Bytes()), 0)
	compositeItem.WaitForHashed()
	if !c.defaultFlags.Quiet {
		fmt.Printf("%s  %s\n", compositeItem.Digest(), compositeName)
	}

	if len(c.dumpHash) != 0 {
		if err = ioutil.WriteFile(c.dumpHash, []byte(compositeItem.Digest()), 0644); err != nil {
			return
		}
	}
	if len(c.dumpIsolated) != 0 {
		if err = ioutil.WriteFile(c.dumpIsolated, rawComposite.Bytes(), 0644); err != nil {
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

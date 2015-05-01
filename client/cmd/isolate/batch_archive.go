// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/luci/luci-go/client/archiver"
	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolate"
	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/luci/luci-go/common/isolated"
	"github.com/maruel/subcommands"
)

var cmdBatchArchive = &subcommands.Command{
	UsageLine: "batcharchive <options> file1 file2 ...",
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
		c := batchArchiveRun{}
		c.commonFlags.Init(&c.CommandRunBase)
		c.commonServerFlags.Init(&c.CommandRunBase)
		c.Flags.StringVar(&c.dumpJson, "dump-json", "",
			"Write isolated Digestes of archived trees to this file as JSON")
		return &c
	},
}

type batchArchiveRun struct {
	subcommands.CommandRunBase
	commonFlags
	commonServerFlags
	dumpJson string
}

func (c *batchArchiveRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonServerFlags.Parse(); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("at least one isolate file required")
	}
	return nil
}

func parseArchiveCMD(args []string, cwd string) (*isolate.ArchiveOptions, error) {
	// Python isolate allows form "--XXXX-variable key value".
	// Golang flag pkg doesn't consider value to be part of --XXXX-variable flag.
	// Therefore, we convert all such "--XXXX-variable key value" to
	// "--XXXX-variable key --XXXX-variable value" form.
	// Note, that key doesn't have "=" in it in either case, but value might.
	// TODO(tandrii): eventually, we want to retire this hack.
	args = convertPyToGoArchiveCMDArgs(args)
	base := subcommands.CommandRunBase{}
	i := isolateFlags{}
	i.Init(&base)
	if err := base.GetFlags().Parse(args); err != nil {
		return nil, err
	}
	if err := i.Parse(RequireIsolatedFile); err != nil {
		return nil, err
	}
	if base.GetFlags().NArg() > 0 {
		return nil, fmt.Errorf("no positional arguments expected")
	}
	return &i.ArchiveOptions, nil
}

// convertPyToGoArchiveCMDArgs converts kv-args from old python isolate into go variants.
// Essentially converts "--X key value" into "--X key=value".
func convertPyToGoArchiveCMDArgs(args []string) []string {
	kvars := map[string]bool{
		"--path-variable": true, "--config-variable": true, "--extra-vriable": true}
	newArgs := []string{}
	for i := 0; i < len(args); {
		newArgs = append(newArgs, args[i])
		kvar := args[i]
		i++
		if !kvars[kvar] {
			continue
		}
		if i >= len(args) {
			// Ignore unexpected behaviour, it'll be caught by flags.Parse() .
			break
		}
		appendArg := args[i]
		i++
		if !strings.Contains(appendArg, "=") && i < len(args) {
			// appendArg is key, and args[i] is value .
			appendArg = fmt.Sprintf("%s=%s", appendArg, args[i])
			i++
		}
		newArgs = append(newArgs, appendArg)
	}
	return newArgs
}

func (c *batchArchiveRun) main(a subcommands.Application, args []string) error {
	start := time.Now()
	arch := archiver.New(isolatedclient.New(c.serverURL, c.namespace))
	common.CancelOnCtrlC(arch)
	type tmp struct {
		name   string
		future archiver.Future
	}
	items := make(chan *tmp, len(args))
	var wg sync.WaitGroup
	for _, arg := range args {
		wg.Add(1)
		go func(genJsonPath string) {
			defer wg.Done()
			data := &struct {
				Args    []string
				Dir     string
				Version int
			}{}
			if err := common.ReadJSONFile(genJsonPath, data); err != nil {
				arch.Cancel(err)
				return
			}
			if data.Version != isolate.IsolatedGenJSONVersion {
				arch.Cancel(fmt.Errorf("invalid version %d in %s", data.Version, genJsonPath))
				return
			}
			if !common.IsDirectory(data.Dir) {
				arch.Cancel(fmt.Errorf("invalid dir %s in %s", data.Dir, genJsonPath))
				return
			}
			opts, err := parseArchiveCMD(data.Args, data.Dir)
			if err != nil {
				arch.Cancel(fmt.Errorf("invalid archive command in %s: %s", genJsonPath, err))
				return
			}
			name := strings.SplitN(filepath.Base(opts.Isolate), ".", 2)[0]
			items <- &tmp{name, isolate.Archive(arch, opts)}
		}(arg)
	}
	go func() {
		wg.Wait()
		close(items)
	}()

	isolatedHashes := map[string]isolated.HexDigest{}
	for item := range items {
		item.future.WaitForHashed()
		if item.future.Error() == nil {
			isolatedHashes[item.name] = item.future.Digest()
			fmt.Printf("%s  %s\n", item.future.Digest(), item.name)
		} else {
			fmt.Fprintf(os.Stderr, "%s  %s\n", item.name, item.future.Error())
		}
	}
	err := arch.Close()
	// Only write the file once upload is confirmed.
	if err == nil && c.dumpJson != "" {
		return common.WriteJSONFile(c.dumpJson, isolatedHashes)
	}
	duration := time.Now().Sub(start)
	stats := arch.Stats()
	fmt.Fprintf(os.Stderr, "Hits    : %5d (%.1fkb)\n", len(stats.Hits), float64(stats.TotalHits())/1024.)
	fmt.Fprintf(os.Stderr, "Misses  : %5d (%.1fkb)\n", len(stats.Misses), float64(stats.TotalMisses())/1024.)
	fmt.Fprintf(os.Stderr, "Pushed  : %5d (%.1fkb)\n", len(stats.Pushed), float64(stats.TotalPushed())/1024.)
	fmt.Fprintf(os.Stderr, "Duration: %s\n", duration)
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

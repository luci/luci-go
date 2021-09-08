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

package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/pprof"
	"strings"
	"sync"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/downloader"
	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/signals"
)

// CmdDownload returns an object for the `download` subcommand.
func CmdDownload(options CommandOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "download <options>...",
		ShortDesc: "downloads a file or a .isolated tree from an isolate server.",
		LongDesc: `Downloads one or multiple files, or a isolated tree from the isolate server.

Files are referenced by their hash`,
		CommandRun: func() subcommands.CommandRun {
			c := downloadRun{
				CommandOptions: options,
			}
			c.commonFlags.Init(options.DefaultAuthOpts)
			c.cachePolicies.AddFlags(&c.Flags)
			// TODO(mknyszek): Add support for downloading individual files.
			c.Flags.StringVar(&c.outputDir, "output-dir", ".", "The directory where files will be downloaded to.")
			c.Flags.StringVar(&c.outputFiles, "output-files", "", "File into which the full list of downloaded files is written to.")
			c.Flags.StringVar(&c.isolated, "isolated", "", "Hash of a .isolated tree to download.")

			c.Flags.StringVar(&c.cacheDir, "cache-dir", "", "Cache directory to store downloaded files.")

			c.Flags.StringVar(&c.resultJSON, "fetch-and-map-result-json", "", "This is created only for crbug.com/932396, do not use other than from run_isolated.py.")
			return &c
		},
	}
}

type downloadRun struct {
	commonFlags
	CommandOptions
	outputDir   string
	outputFiles string
	isolated    string

	resultJSON string

	cacheDir      string
	cachePolicies cache.Policies
}

func (c *downloadRun) Parse(a subcommands.Application, args []string) error {
	if err := c.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.New("position arguments not expected")
	}
	if c.isolated == "" {
		return errors.New("isolated is required")
	}

	if c.cacheDir == "" && !c.cachePolicies.IsDefault() {
		return errors.New("cache-dir is necessary when cache-max-size, cache-max-items or cache-min-free-space are specified")
	}
	return nil
}

type results struct {
	ItemsCold          []byte             `json:"items_cold"`
	ItemsHot           []byte             `json:"items_hot"`
	InitialNumberItems int64              `json:"initial_number_items"`
	InitialSize        int64              `json:"initial_size"`
	Isolated           *isolated.Isolated `json:"isolated"`
}

func (c *downloadRun) outputResults(cache *cache.Cache, initStats initCacheStats, dl *downloader.Downloader) error {
	if c.resultJSON == "" {
		return nil
	}

	itemsCold, itemsHot, err := downloader.CacheStats(cache)
	if err != nil {
		return errors.Annotate(err, "failed to call CacheStats").Err()
	}

	root, err := dl.RootIsolated()
	if err != nil {
		return errors.Annotate(err, "failed to get root isolated").Err()
	}

	resultJSON, err := json.Marshal(results{
		ItemsCold:          itemsCold,
		ItemsHot:           itemsHot,
		InitialNumberItems: initStats.numItems,
		InitialSize:        initStats.totalSize,
		Isolated:           root,
	})
	if err != nil {
		return errors.Annotate(err, "failed to marshal result json").Err()
	}
	if err := ioutil.WriteFile(c.resultJSON, resultJSON, 0664); err != nil {
		return errors.Annotate(err, "failed to write result json to %s", c.resultJSON).Err()
	}

	return nil
}

func (c *downloadRun) main(a subcommands.Application, args []string) error {
	// Prepare isolated client.
	ctx, cancel := context.WithCancel(c.defaultFlags.MakeLoggingContext(os.Stderr))
	defer cancel()
	defer signals.HandleInterrupt(func() {
		pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
		cancel()
	})()
	if err := c.runMain(ctx, a, args); err != nil {
		errors.Log(ctx, err)
		return err
	}
	return nil
}

type initCacheStats struct {
	numItems  int64
	totalSize int64
}

func (c *downloadRun) runMain(ctx context.Context, a subcommands.Application, args []string) error {
	client, err := c.createIsolatedClient(ctx, c.CommandOptions)
	if err != nil {
		return errors.Annotate(err, "failed to create isolated client").Err()
	}
	var filesMu sync.Mutex
	var files []string

	var diskCache *cache.Cache
	var initStats initCacheStats
	if c.cacheDir != "" {
		if err := os.MkdirAll(c.cacheDir, os.ModePerm); err != nil {
			return errors.Annotate(err, "failed to create cache dir: %s", c.cacheDir).Err()
		}
		diskCache, err = cache.New(c.cachePolicies, c.cacheDir, isolated.GetHash(c.isolatedFlags.Namespace))
		if err != nil && diskCache == nil {
			return errors.Annotate(err, "failed to initialize disk cache in %s", c.cacheDir).Err()
		}
		if err != nil {
			logging.WithError(err).Warningf(ctx, "There is (ignorable?) error when initializing disk cache in %s", c.cacheDir)
		}
		defer diskCache.Close()
		initStats.numItems = int64(len(diskCache.Keys()))
		initStats.totalSize = int64(diskCache.TotalSize())
	}

	if err := os.MkdirAll(c.outputDir, os.ModePerm); err != nil {
		return errors.Annotate(err, "failed to create output dir: %s", c.outputDir).Err()
	}

	dl := downloader.New(ctx, client, isolated.HexDigest(c.isolated), c.outputDir, &downloader.Options{
		FileCallback: func(name string, _ *isolated.File) {
			filesMu.Lock()
			files = append(files, name)
			filesMu.Unlock()
		},
		Cache: diskCache,
	})
	if err := dl.Wait(); err != nil {
		return errors.Annotate(err, "failed to call FetchIsolated()").Err()
	}
	if c.outputFiles != "" {
		filesData := strings.Join(files, "\n")
		if len(files) > 0 {
			filesData += "\n"
		}

		if err := ioutil.WriteFile(c.outputFiles, []byte(filesData), 0664); err != nil {
			return errors.Annotate(err, "failed to call WriteFile(%s, ...)", c.outputFiles).Err()
		}
	}

	return c.outputResults(diskCache, initStats, dl)
}

func (c *downloadRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if err := c.Parse(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: failed to call Parse(%s): %v\n", a.GetName(), args, err)
		return 1
	}
	defer c.profilerFlags.Stop()
	if err := c.main(a, args); err != nil {
		fmt.Fprintf(a.GetErr(), "%s: failed to call main(%s): %v\n", a.GetName(), args, err)
		return 1
	}
	return 0
}

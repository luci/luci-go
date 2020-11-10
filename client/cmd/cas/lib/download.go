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
	"crypto"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	isol "go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/system/signals"
)

// CmdDownload returns an object for the `download` subcommand.
func CmdDownload(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "download <options>...",
		ShortDesc: "download directory tree from a CAS server.",
		LongDesc: `Downloads directory tree from the CAS server.

Tree is referenced by their digest "<digest hash>/<size bytes>"`,
		CommandRun: func() subcommands.CommandRun {
			c := downloadRun{}
			c.Init(defaultAuthOpts)
			c.cachePolicies.AddFlags(&c.Flags)
			c.Flags.StringVar(&c.cacheDir, "cache-dir", "", "Cache directory to store downloaded files.")
			c.Flags.StringVar(&c.digest, "digest", "", `Digest of root directory proto "<digest hash>/<size bytes>".`)
			c.Flags.StringVar(&c.dir, "dir", "", "Directory to download tree.")
			c.Flags.StringVar(&c.dumpStatsJSON, "dump-stats-json", "", "Dump download stats to json file.")
			return &c
		},
	}
}

type downloadRun struct {
	commonFlags
	digest        string
	dir           string
	dumpStatsJSON string

	cacheDir      string
	cachePolicies cache.Policies
}

func (r *downloadRun) parse(a subcommands.Application, args []string) error {
	if err := r.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.Reason("position arguments not expected").Err()
	}

	if r.cacheDir == "" && !r.cachePolicies.IsDefault() {
		return errors.New("cache-dir is necessary when cache-max-size, cache-max-items or cache-min-free-space are specified")
	}

	return nil
}

// doDownload downloads directory tree from the CAS server.
func (r *downloadRun) doDownload(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	signals.HandleInterrupt(cancel)

	d, err := digest.NewFromString(r.digest)
	if err != nil {
		return errors.Annotate(err, "failed to parse digest: %s", r.digest).Err()
	}

	c, err := newCasClient(ctx, r.casFlags.Instance, r.parsedAuthOpts, true)
	if err != nil {
		return err
	}

	rootDir := &repb.Directory{}
	if err := c.ReadProto(ctx, d, rootDir); err != nil {
		return errors.Annotate(err, "failed to read root directory proto").Err()
	}

	start := time.Now()
	dirs, err := c.GetDirectoryTree(ctx, d.ToProto())
	if err != nil {
		return errors.Annotate(err, "failed to call GetDirectoryTree").Err()
	}
	logger := logging.Get(ctx)
	logger.Infof("finished GetDirectoryTree api call: %d, took %s", len(dirs), time.Since(start))

	start = time.Now()
	t := &repb.Tree{
		Root:     rootDir,
		Children: dirs,
	}

	outputs, err := c.FlattenTree(t, r.dir)
	if err != nil {
		return errors.Annotate(err, "failed to call FlattenTree").Err()
	}

	to := make(map[digest.Digest]*client.TreeOutput)

	var diskcache *cache.Cache
	if r.cacheDir != "" {
		diskcache, err = cache.New(r.cachePolicies, r.cacheDir, crypto.SHA256)
		if err != nil {
			return errors.Annotate(err, "failed to create initialize cache").Err()
		}
		defer diskcache.Close()
	}

	// Files have the same digest are downloaded only once, so we need to
	// copy duplicates files later.
	var dups []*client.TreeOutput

	mkdirmap := make(map[string]struct{})
	mkdir := func(dir string) error {
		if _, ok := mkdirmap[dir]; ok {
			return nil
		}
		mkdirmap[dir] = struct{}{}
		return os.MkdirAll(dir, 0o700)
	}

	for path, output := range outputs {
		if output.IsEmptyDirectory {
			if err := mkdir(path); err != nil {
				return errors.Annotate(err, "failed to create directory").Err()
			}
			continue
		}

		if err := mkdir(filepath.Dir(path)); err != nil {
			return errors.Annotate(err, "failed to create directory").Err()
		}

		if output.SymlinkTarget != "" {
			if err := os.Symlink(output.SymlinkTarget, path); err != nil {
				return errors.Annotate(err, "failed to create symlink").Err()
			}
			continue
		}

		if diskcache != nil && diskcache.Touch(isolated.HexDigest(output.Digest.Hash)) {
			mode := 0o600
			if output.IsExecutable {
				mode = 0o700
			}

			if err := diskcache.Hardlink(isolated.HexDigest(output.Digest.Hash), path, os.FileMode(mode)); err != nil {
				return err
			}
			continue
		}

		if _, ok := to[output.Digest]; ok {
			dups = append(dups, output)
		} else {
			to[output.Digest] = output
		}
	}
	logger.Infof("finished copy from cache (if any), dups: %d, to: %d, took %s", len(dirs), len(to), time.Since(start))

	start = time.Now()
	if err := c.DownloadFiles(ctx, "", to); err != nil {
		return errors.Annotate(err, "failed to download files").Err()
	}
	logger.Infof("finished DownloadFiles api call, took %s", time.Since(start))

	if diskcache != nil {
		start = time.Now()
		outputs := make([]*client.TreeOutput, 0, len(to))
		for _, output := range to {
			outputs = append(outputs, output)
		}

		// This is to utilize locality of disk access.
		sort.Slice(outputs, func(i, j int) bool {
			return outputs[i].Path < outputs[j].Path
		})

		for _, output := range outputs {
			if err := diskcache.AddFileWithoutValidation(
				isolated.HexDigest(output.Digest.Hash), output.Path); err != nil {
				return errors.Annotate(err, "failed to add cache; path=%s digest=%s", output.Path, output.Digest).Err()
			}
		}
		logger.Infof("finished cache addition, took %s", time.Since(start))
	}

	start = time.Now()
	eg, _ := errgroup.WithContext(ctx)

	// limit the number of concurrent I/O threads.
	ch := make(chan struct{}, runtime.NumCPU())

	for _, dup := range dups {
		src := to[dup.Digest]
		dst := dup
		ch <- struct{}{}
		eg.Go(func() (err error) {
			defer func() { <-ch }()
			mode := 0o600
			if dst.IsExecutable {
				mode = 0o700
			}

			if err := filesystem.Copy(dst.Path, src.Path, os.FileMode(mode)); err != nil {
				return errors.Annotate(err, "failed to copy file from '%s' to '%s'", src.Path, dst.Path).Err()
			}

			return nil
		})
	}
	err = eg.Wait()

	logger.Infof("finished files copy of %d, took %s", len(dups), time.Since(start))

	if dsj := r.dumpStatsJSON; dsj != "" {
		cold := make([]int64, 0, len(to))
		for d := range to {
			cold = append(cold, d.Size)
		}
		hot := make([]int64, 0, len(outputs)-len(to))
		for _, output := range outputs {
			d := output.Digest
			if _, ok := to[d]; !ok {
				hot = append(hot, d.Size)
			}
		}

		if err := isol.WriteStats(dsj, hot, cold); err != nil {
			return errors.Annotate(err, "failed to write stats json").Err()
		}
	}

	return err
}

func (r *downloadRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	logging.Get(ctx).Infof("start command")
	if err := r.parse(a, args); err != nil {
		errors.Log(ctx, err)
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}
	defer r.profiler.Stop()

	if err := r.doDownload(ctx); err != nil {
		errors.Log(ctx, err)
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}

	return 0
}

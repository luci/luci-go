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
	"crypto"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/system/signals"
)

const smallFileThreshold = 16 * 1024 // 16KiB

// CmdDownload returns an object for the `download` subcommand.
func CmdDownload(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "download <options>...",
		ShortDesc: "download directory tree from a CAS server.",
		LongDesc: `Downloads directory tree from the CAS server.

Tree is referenced by their digest "<digest hash>/<size bytes>"`,
		CommandRun: func() subcommands.CommandRun {
			c := downloadRun{}
			c.Init(authFlags)
			c.cachePolicies.AddFlags(&c.Flags)
			c.Flags.StringVar(&c.cacheDir, "cache-dir", "", "Cache directory to store downloaded files.")
			c.Flags.StringVar(&c.digest, "digest", "", `Digest of root directory proto "<digest hash>/<size bytes>".`)
			c.Flags.StringVar(&c.dir, "dir", "", "Directory to download tree.")
			c.Flags.StringVar(&c.dumpJSON, "dump-json", "", "Dump download stats to json file.")
			if newSmallFileCache != nil {
				c.Flags.StringVar(&c.kvs, "kvs-dir", "", "Cache dir for small files.")
			}
			return &c
		},
	}
}

type downloadRun struct {
	commonFlags
	digest        string
	dir           string
	dumpJSON      string
	cacheDir      string
	cachePolicies cache.Policies

	kvs string
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

	if r.kvs != "" && r.cacheDir == "" {
		return errors.New("if kvs-dir is set, cache-dir should be set")
	}

	r.dir = filepath.Clean(r.dir)

	return nil
}

func extractErrorCode(err error) (StatusCode, string) {
	errorCode := RPCError
	digest := ""
	if e, ok := status.FromError(err); ok {
		if e.Code() == codes.PermissionDenied {
			errorCode = AuthenticationError
		} else if e.Code() == codes.NotFound {
			errorCode = DigestInvalid
		}
		re := regexp.MustCompile(`Digest ([0-9a-z]*/\d*) not found in the CAS`)
		parts := re.FindStringSubmatch(e.Message())
		if parts != nil {
			digest = parts[1]
		}
	}
	return errorCode, digest
}

func createDirectories(ctx context.Context, root string, outputs map[string]*client.TreeOutput) error {
	logger := logging.Get(ctx)

	start := time.Now()

	dirset := make(map[string]struct{})

	// Extract unique directory paths for optimization.
	for path, output := range outputs {
		var dir string
		if output.IsEmptyDirectory {
			dir = path
		} else {
			dir = filepath.Dir(path)
		}

		for dir != root {
			if _, ok := dirset[dir]; ok {
				break
			}
			dirset[dir] = struct{}{}
			dir = filepath.Dir(dir)
		}
	}

	dirs := make([]string, 0, len(dirset))
	for dir := range dirset {
		dirs = append(dirs, dir)
	}

	sort.Strings(dirs)

	logger.Infof("preprocess took %s", time.Since(start))
	start = time.Now()

	if err := os.MkdirAll(root, 0o700); err != nil {
		return errors.Annotate(err, "failed to create root dir").Err()
	}

	for _, dir := range dirs {
		if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
			return errors.Annotate(err, "failed to create directory").Err()
		}
	}

	logger.Infof("dir creation took %s", time.Since(start))

	return nil
}

func copyFiles(ctx context.Context, dsts []*client.TreeOutput, srcs map[digest.Digest]*client.TreeOutput) error {
	eg, _ := errgroup.WithContext(ctx)

	// limit the number of concurrent I/O operations.
	ch := make(chan struct{}, runtime.NumCPU())

	for _, dst := range dsts {
		dst := dst
		src := srcs[dst.Digest]
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

	return eg.Wait()
}

type smallFileCache interface {
	Close() error
	GetMulti(context.Context, []string, func(string, []byte) error) error
	SetMulti(func(func(string, []byte) error) error) error
}

var newSmallFileCache func(context.Context, string) (smallFileCache, error)

func copySmallFilesFromCache(ctx context.Context, kvs smallFileCache, smallFiles map[string][]*client.TreeOutput) error {
	smallFileHashes := make([]string, 0, len(smallFiles))
	for smallFile := range smallFiles {
		smallFileHashes = append(smallFileHashes, smallFile)
	}

	var mu sync.Mutex

	// limit the number of concurrent I/O operations.
	ch := make(chan struct{}, runtime.NumCPU())

	// Sort hashes by one of corresponding file path.
	sort.Slice(smallFileHashes, func(i, j int) bool {
		filei := smallFiles[smallFileHashes[i]][0]
		filej := smallFiles[smallFileHashes[j]][0]
		return filei.Path < filej.Path
	})

	// Extract small files from kvs.
	return kvs.GetMulti(ctx, smallFileHashes, func(key string, value []byte) error {
		ch <- struct{}{}
		defer func() { <-ch }()

		mu.Lock()
		files := smallFiles[key]
		delete(smallFiles, key)
		mu.Unlock()

		for _, file := range files {
			mode := 0o600
			if file.IsExecutable {
				mode = 0o700
			}
			if err := os.WriteFile(file.Path, value, os.FileMode(mode)); err != nil {
				return errors.Annotate(err, "failed to write file").Err()
			}
		}

		return nil
	})
}

func cacheSmallFiles(ctx context.Context, kvs smallFileCache, outputs []*client.TreeOutput) error {
	// limit the number of concurrent I/O operations.
	ch := make(chan struct{}, runtime.NumCPU())

	return kvs.SetMulti(func(set func(key string, value []byte) error) error {
		var eg errgroup.Group

		for _, output := range outputs {
			output := output

			eg.Go(func() error {
				b, err := func() ([]byte, error) {
					ch <- struct{}{}
					defer func() { <-ch }()
					return os.ReadFile(output.Path)
				}()

				if err != nil {
					return errors.Annotate(err, "failed to read file: %s", output.Path).Err()
				}
				return set(output.Digest.Hash, b)
			})
		}

		return eg.Wait()
	})
}

func cacheOutputFiles(ctx context.Context, diskcache *cache.Cache, kvs smallFileCache, outputs map[digest.Digest]*client.TreeOutput) error {
	var smallOutputs, largeOutputs []*client.TreeOutput

	for _, output := range outputs {
		if kvs != nil && output.Digest.Size <= smallFileThreshold {
			smallOutputs = append(smallOutputs, output)
		} else {
			largeOutputs = append(largeOutputs, output)
		}
	}

	// This is to utilize locality of disk access.
	sort.Slice(smallOutputs, func(i, j int) bool {
		return smallOutputs[i].Path < smallOutputs[j].Path
	})

	sort.Slice(largeOutputs, func(i, j int) bool {
		return largeOutputs[i].Path < largeOutputs[j].Path
	})

	logger := logging.Get(ctx)

	if kvs != nil {
		start := time.Now()
		if err := cacheSmallFiles(ctx, kvs, smallOutputs); err != nil {
			return err
		}
		logger.Infof("finished cacheSmallFiles %d, took %s", len(smallOutputs), time.Since(start))
	}

	start := time.Now()
	for _, output := range largeOutputs {
		if err := diskcache.AddFileWithoutValidation(ctx, cache.HexDigest(output.Digest.Hash), output.Path); err != nil {
			return errors.Annotate(err, "failed to add cache; path=%s digest=%s", output.Path, output.Digest).Err()
		}
	}
	logger.Infof("finished cache large files %d, took %s", len(largeOutputs), time.Since(start))

	return nil
}

// doDownload downloads directory tree from the CAS server.
func (r *downloadRun) doDownload(ctx context.Context) (rerr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer signals.HandleInterrupt(cancel)()
	ctx, err := casclient.ContextWithMetadata(ctx, "cas")
	if err != nil {
		return err
	}

	d, err := digest.NewFromString(r.digest)
	if err != nil {
		if err := writeExitResult(r.dumpJSON, DigestInvalid, r.digest); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return errors.Annotate(err, "failed to parse digest: %s", r.digest).Err()
	}

	c, err := r.authFlags.NewRBEClient(ctx, r.casFlags.Addr, r.casFlags.Instance, true)
	if err != nil {
		if err := writeExitResult(r.dumpJSON, ClientError, ""); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return err
	}
	rootDir := &repb.Directory{}
	if _, err := c.ReadProto(ctx, d, rootDir); err != nil {
		errorCode, digest := extractErrorCode(err)
		if err := writeExitResult(r.dumpJSON, errorCode, digest); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return errors.Annotate(err, "failed to read root directory proto").Err()
	}

	start := time.Now()
	dirs, err := c.GetDirectoryTree(ctx, d.ToProto())
	if err != nil {
		if err := writeExitResult(r.dumpJSON, RPCError, ""); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
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
		errorCode, digest := extractErrorCode(err)
		if err := writeExitResult(r.dumpJSON, errorCode, digest); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return errors.Annotate(err, "failed to call FlattenTree").Err()
	}

	to := make(map[digest.Digest]*client.TreeOutput)

	var diskcache *cache.Cache
	if r.cacheDir != "" {
		// Increase free space with to be downloaded file size.
		for _, output := range outputs {
			if output.IsEmptyDirectory || output.SymlinkTarget != "" {
				continue
			}
			r.cachePolicies.MinFreeSpace += units.Size(output.Digest.Size)
		}

		diskcache, err = cache.New(r.cachePolicies, r.cacheDir, crypto.SHA256)
		if err != nil {
			if err := writeExitResult(r.dumpJSON, IOError, ""); err != nil {
				return errors.Annotate(err, "failed to write json file").Err()
			}
			return errors.Annotate(err, "failed to create initialize cache").Err()
		}
		defer diskcache.Close()
	}

	var kvs smallFileCache
	if r.kvs != "" {
		kvs, err = newSmallFileCache(ctx, r.kvs)
		if err != nil {
			return err
		}
		defer func() {
			start := time.Now()
			defer func() {
				logger.Infof("closing kvs, took %s", time.Since(start))
			}()
			if err := kvs.Close(); err != nil {
				logger.Errorf("failed to close kvs cache: %v", err)
				if rerr == nil {
					rerr = errors.Annotate(err, "failed to close kvs cache").Err()
				}
			}
		}()
	}

	if err := createDirectories(ctx, r.dir, outputs); err != nil {
		return err
	}
	logger.Infof("finish createDirectories, took %s", time.Since(start))
	start = time.Now()

	// Files have the same digest are downloaded only once, so we need to
	// copy duplicates files later.
	var dups []*client.TreeOutput

	smallFiles := make(map[string][]*client.TreeOutput)

	sortedPaths := make([]string, 0, len(outputs))
	for path := range outputs {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	for _, path := range sortedPaths {
		output := outputs[path]
		if output.IsEmptyDirectory {
			continue
		}

		if output.SymlinkTarget != "" {
			if err := os.Symlink(output.SymlinkTarget, path); err != nil {
				if err := writeExitResult(r.dumpJSON, IOError, ""); err != nil {
					return errors.Annotate(err, "failed to write json file").Err()
				}
				return errors.Annotate(err, "failed to create symlink").Err()
			}
			continue
		}

		if kvs != nil && output.Digest.Size <= smallFileThreshold {
			smallFiles[output.Digest.Hash] = append(smallFiles[output.Digest.Hash], output)
			continue
		}

		if diskcache != nil && diskcache.Touch(cache.HexDigest(output.Digest.Hash)) {
			mode := 0o600
			if output.IsExecutable {
				mode = 0o700
			}

			if err := diskcache.Hardlink(cache.HexDigest(output.Digest.Hash), path, os.FileMode(mode)); err != nil {
				if err := writeExitResult(r.dumpJSON, IOError, ""); err != nil {
					return errors.Annotate(err, "failed to write json file").Err()
				}
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
	logger.Infof("finished copy from cache (if any), dups: %d, to: %d, smallFiles: %d, took %s",
		len(dups), len(to), len(smallFiles), time.Since(start))

	if kvs != nil {
		start := time.Now()

		if err := copySmallFilesFromCache(ctx, kvs, smallFiles); err != nil {
			if err := writeExitResult(r.dumpJSON, IOError, ""); err != nil {
				return errors.Annotate(err, "failed to write json file").Err()
			}
			return err
		}

		// Process non-cached files.
		for _, files := range smallFiles {
			for _, file := range files {
				if _, ok := to[file.Digest]; ok {
					dups = append(dups, file)
				} else {
					to[file.Digest] = file
				}
			}
		}

		logger.Infof("finished copy small files from cache (if any), to: %d, took %s", len(to), time.Since(start))
	}

	start = time.Now()
	if _, err := c.DownloadFiles(ctx, "", to); err != nil {
		errorCode, digest := extractErrorCode(err)
		if err := writeExitResult(r.dumpJSON, errorCode, digest); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return errors.Annotate(err, "failed to download files").Err()
	}
	logger.Infof("finished DownloadFiles api call, took %s", time.Since(start))

	if diskcache != nil {
		start = time.Now()
		if err := cacheOutputFiles(ctx, diskcache, kvs, to); err != nil {
			if err := writeExitResult(r.dumpJSON, IOError, ""); err != nil {
				return errors.Annotate(err, "failed to write json file").Err()
			}
			return err
		}
		logger.Infof("finished cache addition, took %s", time.Since(start))
	}

	start = time.Now()
	if err := copyFiles(ctx, dups, to); err != nil {
		if err := writeExitResult(r.dumpJSON, IOError, ""); err != nil {
			return errors.Annotate(err, "failed to write json file").Err()
		}
		return err
	}
	logger.Infof("finished files copy of %d, took %s", len(dups), time.Since(start))

	if dsj := r.dumpJSON; dsj != "" {
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

		if err := writeStats(dsj, hot, cold); err != nil {
			return errors.Annotate(err, "failed to write stats json").Err()
		}
	}

	return nil
}

func (r *downloadRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	logging.Infof(ctx, "Starting %s", Version)

	if err := r.parse(a, args); err != nil {
		errors.Log(ctx, err)
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		if err := writeExitResult(r.dumpJSON, ArgumentsInvalid, ""); err != nil {
			fmt.Fprintf(a.GetErr(), "failed to write json file")
		}
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

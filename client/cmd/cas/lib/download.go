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
	"os"
	"path/filepath"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/tree"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/system/signals"
)

// CmdDownload returns an object for the `download` subcommand.
func CmdDownload() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "download <options>...",
		ShortDesc: "download directory tree from a CAS server.",
		LongDesc: `Downloads directory tree from the CAS server.

Tree is referenced by their digest "<digest hash>/<size bytes>"`,
		CommandRun: func() subcommands.CommandRun {
			c := downloadRun{}
			c.Init()
			c.Flags.StringVar(&c.digest, "digest", "", `Digest of root directory proto "<digest hash>/<size bytes>".`)
			c.Flags.StringVar(&c.dir, "dir", "", "Directory to download tree.")
			return &c
		},
	}
}

type downloadRun struct {
	commonFlags
	digest string
	dir    string
}

func (r *downloadRun) parse(a subcommands.Application, args []string) error {
	if err := r.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) != 0 {
		return errors.Reason("position arguments not expected").Err()
	}
	return nil
}

// doDownload downloads directory tree from the CAS server.
func (r *downloadRun) doDownload(ctx context.Context, args []string) error {
	ctx, cancel := context.WithCancel(ctx)
	signals.HandleInterrupt(cancel)

	d, err := digest.NewFromString(r.digest)
	if err != nil {
		return errors.Annotate(err, "failed to parse digest: %s", r.digest).Err()
	}

	c, err := r.casFlags.NewClient(ctx, true)
	if err != nil {
		return err
	}

	rootDir := &repb.Directory{}
	if err := c.ReadProto(ctx, d, rootDir); err != nil {
		return errors.Annotate(err, "failed to read root directory proto").Err()
	}

	dirs, err := c.GetDirectoryTree(ctx, d.ToProto())
	if err != nil {
		return errors.Annotate(err, "failed to call GetDirectoryTree").Err()
	}

	t := &repb.Tree{
		Root:     rootDir,
		Children: dirs,
	}

	outputs, err := tree.FlattenTree(t, r.dir)
	if err != nil {
		return errors.Annotate(err, "failed to call FlattenTree").Err()
	}

	to := make(map[digest.Digest]*tree.Output)

	// Files have the same digest are downloaded only once, so we need to
	// copy duplicates files later.
	var dups []*tree.Output

	for path, output := range outputs {
		if output.IsEmptyDirectory {
			if err := os.MkdirAll(path, 0o700); err != nil {
				return errors.Annotate(err, "failed to create directory").Err()
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return errors.Annotate(err, "failed to create directory").Err()
		}

		if output.SymlinkTarget != "" {
			if err := os.Symlink(output.SymlinkTarget, path); err != nil {
				return errors.Annotate(err, "failed to create symlink").Err()
			}
			continue
		}

		if _, ok := to[output.Digest]; ok {
			dups = append(dups, output)
		} else {
			to[output.Digest] = output
		}
	}

	if err := c.DownloadFiles(ctx, ".", to); err != nil {
		return errors.Annotate(err, "failed to download files").Err()
	}

	// DownloadFiles does not set desired file permission.
	for _, output := range to {
		mode := 0o600
		if output.IsExecutable {
			mode = 0o700
		}
		if err := os.Chmod(output.Path, os.FileMode(mode)); err != nil {
			return errors.Annotate(err, "failed to change mode").Err()
		}
	}

	eg, ctx := errgroup.WithContext(ctx)

	for _, dup := range dups {
		src := to[dup.Digest]
		dst := dup
		eg.Go(func() (err error) {
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

func (r *downloadRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parse(a, args); err != nil {
		errors.Log(ctx, err)
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}

	if err := r.doDownload(ctx, args); err != nil {
		errors.Log(ctx, err)
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}

	return 0
}

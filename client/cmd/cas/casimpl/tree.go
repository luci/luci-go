// Copyright 2026 The LUCI Authors.
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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/signals"
)

const (
	defaultCacheMaxSize  = 50 * 1024 * 1024 // 50 MiB
	defaultCacheMaxItems = 1000
)

// TreeFileEntry represents metadata of a file or node in the directory tree.
type TreeFileEntry struct {
	Digest        string `json:"digest,omitempty"`
	Size          int64  `json:"size,omitempty"`
	IsExecutable  bool   `json:"is_executable,omitempty"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
	IsEmptyDir    bool   `json:"is_empty_dir,omitempty"`
}

// TreeResult represents the complete metadata tree of a CAS root directory.
type TreeResult struct {
	Digest     string                   `json:"digest"`
	Files      map[string]TreeFileEntry `json:"files"`
	TotalFiles int                      `json:"total_files"`
	TotalBytes int64                    `json:"total_bytes"`
}

// CmdTree returns an object for the `tree` subcommand.
func CmdTree(authFlags AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "tree <options>...",
		ShortDesc: "retrieve directory tree metadata from a CAS server.",
		LongDesc: `Retrieves full directory tree metadata from the CAS server without downloading file contents.

Tree metadata is output in JSON format (to stdout or written to -destination).
Tree is referenced by its root directory digest "<digest hash>/<size bytes>".`,
		CommandRun: func() subcommands.CommandRun {
			c := treeRun{}
			c.Init(authFlags)
			c.cachePolicies.AddFlags(&c.Flags)
			c.Flags.StringVar(&c.cacheDir, "cache-dir", "", "Cache directory to store retrieved tree metadata.")
			c.Flags.StringVar(&c.destination, "destination", "", "Path to write tree metadata JSON (Defaults to stdout if unspecified).")
			c.Flags.StringVar(&c.digest, "digest", "", `Digest of root directory proto "<digest hash>/<size bytes>".`)
			return &c
		},
	}
}

type treeRun struct {
	commonFlags
	destination   string
	digest        string
	cacheDir      string
	cachePolicies cache.Policies

	diskcache *cache.Cache
	stdout    io.Writer // for testing; defaults to os.Stdout if nil
}

func (r *treeRun) parse(a subcommands.Application, args []string) error {
	if err := r.commonFlags.Parse(); err != nil {
		return err
	}
	if len(args) == 1 && r.digest == "" {
		r.digest = args[0]
	} else if len(args) != 0 {
		return errors.New("position arguments not expected")
	}

	if r.digest == "" {
		return errors.New("-digest is required")
	}

	if r.cacheDir == "" && !r.cachePolicies.IsDefault() {
		return errors.New("cache-dir is necessary when cache-max-size, cache-max-items or cache-min-free-space are specified")
	}

	return nil
}

func (r *treeRun) doTree(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer signals.HandleInterrupt(cancel)()
	ctx, err := casclient.ContextWithMetadata(ctx, "cas")
	if err != nil {
		return err
	}

	d, err := digest.NewFromString(r.digest)
	if err != nil {
		if err := writeExitResult(r.destination, DigestInvalid, r.digest); err != nil {
			return errors.Fmt("write json file: %w", err)
		}
		return errors.Fmt("parse digest %s: %w", r.digest, err)
	}

	if cachedTreeResult, ok := r.readTreeFromCache(d); ok {
		defer r.diskcache.Close()
		logging.Infof(ctx, "cache hit for tree digest: %s", d)
		return r.outputResult(cachedTreeResult)
	}
	if r.cacheDir != "" && r.diskcache == nil {
		return errors.New("initialize cache")
	}
	if r.diskcache != nil {
		defer r.diskcache.Close()
	}

	c, err := r.newRBEClient(ctx, r.destination, true)
	if err != nil {
		return err
	}
	defer c.Close()

	outputs, err := fetchAndFlattenTree(ctx, c, d, r.destination)
	if err != nil {
		return err
	}

	treeResult := buildTreeResult(d, outputs)
	r.writeTreeToCache(ctx, d, treeResult)

	return r.outputResult(treeResult)
}

func buildTreeResult(d digest.Digest, outputs map[string]*client.TreeOutput) *TreeResult {
	res := &TreeResult{
		Digest: d.String(),
		Files:  make(map[string]TreeFileEntry, len(outputs)),
	}

	for path, out := range outputs {
		entry := TreeFileEntry{}
		switch {
		case out.IsEmptyDirectory:
			entry.IsEmptyDir = true
		case out.SymlinkTarget != "":
			entry.SymlinkTarget = filepath.ToSlash(out.SymlinkTarget)
		default:
			entry.Digest = out.Digest.String()
			entry.Size = out.Digest.Size
			entry.IsExecutable = out.IsExecutable
			res.TotalFiles++
			res.TotalBytes += out.Digest.Size
		}
		res.Files[filepath.ToSlash(path)] = entry
	}
	return res
}

func (r *treeRun) readTreeFromCache(d digest.Digest) (*TreeResult, bool) {
	if r.cacheDir == "" {
		return nil, false
	}
	policies := r.cachePolicies
	if policies.MaxSize == 0 {
		policies.MaxSize = units.Size(defaultCacheMaxSize)
	}
	if policies.MaxItems == 0 {
		policies.MaxItems = defaultCacheMaxItems
	}
	c, err := initCache(r.cacheDir, policies, r.destination)
	if err != nil || c == nil {
		return nil, false
	}
	r.diskcache = c

	hexDigest := cache.HexDigest(d.Hash)
	if !hexDigest.Validate(crypto.SHA256) {
		return nil, false
	}
	reader, err := r.diskcache.Read(hexDigest)
	if err != nil {
		return nil, false
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, false
	}
	var res TreeResult
	if err := json.Unmarshal(data, &res); err != nil {
		r.diskcache.Evict(hexDigest)
		return nil, false
	}
	return &res, true
}

func (r *treeRun) writeTreeToCache(ctx context.Context, d digest.Digest, res *TreeResult) {
	if r.diskcache == nil {
		return
	}
	hexDigest := cache.HexDigest(d.Hash)
	if !hexDigest.Validate(crypto.SHA256) {
		return
	}
	jsonBytes, err := json.Marshal(res)
	if err != nil {
		return
	}
	tmpFile, err := os.CreateTemp(r.cacheDir, "tree_*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(jsonBytes); err != nil {
		_ = tmpFile.Close()
		return
	}
	if err := tmpFile.Close(); err != nil {
		return
	}
	if err := r.diskcache.AddFileWithoutValidation(ctx, hexDigest, tmpPath); err != nil {
		logging.Warningf(ctx, "cache tree result: %v", err)
	}
}

func (r *treeRun) outputResult(res *TreeResult) error {
	jsonBytes, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return errors.Fmt("marshal json: %w", err)
	}

	if r.destination != "" {
		if err := os.WriteFile(r.destination, jsonBytes, 0600); err != nil {
			return errors.Fmt("write -destination: %w", err)
		}
		return nil
	}

	fmt.Fprintf(r.getStdout(), "%s\n", jsonBytes)
	return nil
}

func (r *treeRun) getStdout() io.Writer {
	if r.stdout != nil {
		return r.stdout
	}
	return os.Stdout
}

func (r *treeRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	logging.Infof(ctx, "Starting %s", Version)

	if err := r.parse(a, args); err != nil {
		errors.Log(ctx, err)
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		if err := writeExitResult(r.destination, ArgumentsInvalid, ""); err != nil {
			fmt.Fprintf(a.GetErr(), "write json file: %s\n", err)
		}
		return 1
	}
	defer r.profiler.Stop()

	if err := r.doTree(ctx); err != nil {
		errors.Log(ctx, err)
		fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
		return 1
	}

	return 0
}

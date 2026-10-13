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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/signals"
)

const (
	defaultCacheMaxSize  = 500 * 1024 * 1024 // 500 MiB
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

// compactTreeFileEntry is the compact JSON representation of TreeFileEntry used with -compact.
// Size is omitted because it is already encoded in Digest ("<hash>/<size>").
type compactTreeFileEntry struct {
	Digest        string `json:"d,omitempty"`
	IsExecutable  bool   `json:"x,omitempty"`
	SymlinkTarget string `json:"l,omitempty"`
	IsEmptyDir    bool   `json:"e,omitempty"`
}

// TreeResult represents the complete metadata tree of a CAS root directory.
type TreeResult struct {
	Digest     string                   `json:"digest"`
	Files      map[string]TreeFileEntry `json:"files"`
	TotalFiles int                      `json:"total_files"`
	TotalBytes int64                    `json:"total_bytes"`
}

type compactTreeResult struct {
	Digest     string                          `json:"digest"`
	Files      map[string]compactTreeFileEntry `json:"files"`
	TotalFiles int                             `json:"total_files"`
	TotalBytes int64                           `json:"total_bytes"`
}

func (res *TreeResult) marshal(compact bool) ([]byte, error) {
	if !compact {
		return json.MarshalIndent(res, "", "  ")
	}
	files := make(map[string]compactTreeFileEntry, len(res.Files))
	for k, v := range res.Files {
		files[k] = compactTreeFileEntry{
			Digest:        v.Digest,
			IsExecutable:  v.IsExecutable,
			SymlinkTarget: v.SymlinkTarget,
			IsEmptyDir:    v.IsEmptyDir,
		}
	}
	return json.Marshal(&compactTreeResult{
		Digest:     res.Digest,
		Files:      files,
		TotalFiles: res.TotalFiles,
		TotalBytes: res.TotalBytes,
	})
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
			c.Flags.BoolVar(&c.compact, "compact", false, "Output compact JSON without indentation and with shortened entry keys.")
			c.Flags.Var(luciflag.StringSlice(&c.exclude), "exclude", "Regular expression of paths to exclude from the tree (repeatable).")
			return &c
		},
	}
}

type treeRun struct {
	commonFlags
	destination    string
	digest         string
	cacheDir       string
	cachePolicies  cache.Policies
	compact        bool
	exclude        []string
	excludeRegexps []*regexp.Regexp

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

	// Compile each -exclude pattern individually rather than joining with "|",
	// preserving Go regexp's LiteralPrefix() SIMD fast-path on each pattern.
	r.excludeRegexps = make([]*regexp.Regexp, 0, len(r.exclude))
	for _, pat := range r.exclude {
		re, err := regexp.Compile(pat)
		if err != nil {
			return errors.Fmt("invalid -exclude regexp %q: %w", pat, err)
		}
		r.excludeRegexps = append(r.excludeRegexps, re)
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

	if cachedJSON, ok := r.readTreeFromCache(d); ok {
		defer r.diskcache.Close()
		logging.Infof(ctx, "cache hit for tree digest: %s", d)
		return r.outputResult(cachedJSON)
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

	treeResult := buildTreeResult(d, outputs, r.excludeRegexps)
	jsonBytes, err := treeResult.marshal(r.compact)
	if err != nil {
		return errors.Fmt("marshal json: %w", err)
	}

	r.writeTreeToCache(ctx, d, jsonBytes)

	return r.outputResult(jsonBytes)
}

func matchesExclude(relPath string, exclude []*regexp.Regexp) bool {
	for _, re := range exclude {
		if re.MatchString(relPath) {
			return true
		}
	}
	return false
}

func buildTreeResult(d digest.Digest, outputs map[string]*client.TreeOutput, exclude []*regexp.Regexp) *TreeResult {
	initialCap := len(outputs)
	if len(exclude) > 0 {
		initialCap = 0
	}
	res := &TreeResult{
		Digest: d.String(),
		Files:  make(map[string]TreeFileEntry, initialCap),
	}

	for path, out := range outputs {
		relPath := filepath.ToSlash(path)
		if matchesExclude(relPath, exclude) {
			continue
		}
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
		res.Files[relPath] = entry
	}
	return res
}

// cacheKeyDigest hashes the root digest with -compact and -exclude options into a SHA-256 cache key.
func (r *treeRun) cacheKeyDigest(d digest.Digest) cache.HexDigest {
	if len(r.exclude) == 0 && !r.compact {
		return cache.HexDigest(d.Hash)
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\ncompact=%t\nexclude=%q", d.Hash, r.compact, r.exclude)
	return cache.HexDigest(hex.EncodeToString(h.Sum(nil)))
}

func (r *treeRun) readTreeFromCache(d digest.Digest) ([]byte, bool) {
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

	hexDigest := r.cacheKeyDigest(d)
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
	if !json.Valid(data) {
		r.diskcache.Evict(hexDigest)
		return nil, false
	}
	return data, true
}

func (r *treeRun) writeTreeToCache(ctx context.Context, d digest.Digest, jsonBytes []byte) {
	if r.diskcache == nil {
		return
	}
	hexDigest := r.cacheKeyDigest(d)
	if !hexDigest.Validate(crypto.SHA256) {
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

func (r *treeRun) outputResult(jsonBytes []byte) error {
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

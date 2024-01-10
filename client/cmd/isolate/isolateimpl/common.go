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

package isolateimpl

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/cas"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/text/units"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/profiling"
	"go.chromium.org/luci/common/system/filesystem"
)

type baseCommandRun struct {
	subcommands.CommandRunBase
	defaultFlags common.Flags
	logConfig    logging.Config // for -log-level, used by ModifyContext
	profiler     profiling.Profiler

	// Overriden in tests.

	clientFactory func(ctx context.Context, addr string, instance string, opts auth.Options, readOnly bool) (*cas.Client, error)
}

var _ cli.ContextModificator = (*baseCommandRun)(nil)

func (c *baseCommandRun) Init() {
	c.defaultFlags.Init(&c.Flags)
	c.logConfig.Level = logging.Warning
	c.logConfig.AddFlags(&c.Flags)
	c.profiler.AddFlags(&c.Flags)
}

func (c *baseCommandRun) Parse() error {
	if c.logConfig.Level == logging.Debug {
		// extract glog flag used in remote-apis-sdks
		logtostderr := flag.Lookup("logtostderr")
		if logtostderr == nil {
			return errors.Reason("logtostderr flag for glog not found").Err()
		}
		v := flag.Lookup("v")
		if v == nil {
			return errors.Reason("v flag for glog not found").Err()
		}
		logtostderr.Value.Set("true")
		v.Value.Set("9")
	}
	if err := c.profiler.Start(); err != nil {
		return err
	}
	return c.defaultFlags.Parse()
}

// ModifyContext implements cli.ContextModificator.
func (c *baseCommandRun) ModifyContext(ctx context.Context) context.Context {
	return c.logConfig.Set(ctx)
}

func (c *baseCommandRun) newClient(ctx context.Context, addr string, instance string, opts auth.Options, readOnly bool) (*cas.Client, error) {
	factory := c.clientFactory
	if factory == nil {
		factory = casclient.New
	}
	return factory(ctx, addr, instance, opts, readOnly)
}

type commonServerFlags struct {
	baseCommandRun
	authFlags authcli.Flags

	parsedAuthOpts auth.Options
}

func (c *commonServerFlags) Init(authOpts auth.Options) {
	c.baseCommandRun.Init()
	c.authFlags.Register(&c.Flags, authOpts)
}

func (c *commonServerFlags) Parse() error {
	var err error
	if err = c.baseCommandRun.Parse(); err != nil {
		return err
	}
	c.parsedAuthOpts, err = c.authFlags.Options()
	return err
}

func (c *commonServerFlags) createAuthClient(ctx context.Context) (*http.Client, error) {
	// Don't enforce authentication by using OptionalLogin mode. This is needed
	// for IP-allowed bots: they have NO credentials to send.
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, c.parsedAuthOpts).Client()
}

type isolateFlags struct {
	// TODO(tandrii): move ArchiveOptions from isolate pkg to here.
	isolate.ArchiveOptions
}

func (c *isolateFlags) Init(f *flag.FlagSet) {
	c.ArchiveOptions.Init()
	f.StringVar(&c.Isolate, "isolate", "", ".isolate file to load the dependency data from")
	f.StringVar(&c.Isolate, "i", "", "Alias for -isolate")
	f.StringVar(&c.IgnoredPathFilterRe, "ignored-path-filter-re", "", "A regular expression for filtering away the paths to be ignored. Note that this regexp matches ANY part of the path. So if you want to match the beginning of a path, you need to explicitly prepend ^ (same for $). Please use the Go regexp syntax. I.e. use double backslack \\\\ if you need a backslash literal.")
	f.Var(&c.ConfigVariables, "config-variable", "Config variables are used to determine which conditions should be matched when loading a .isolate file, default: [].")
	f.Var(&c.PathVariables, "path-variable", "Path variables are used to replace file paths when loading a .isolate file, default: {}")
	f.BoolVar(&c.AllowMissingFileDir, "allow-missing-file-dir", false, "If this flag is true, invalid entries in the isolated file are only logged, but won't stop it from being processed.")
}

func (c *isolateFlags) Parse(cwd string) error {
	if !filepath.IsAbs(cwd) {
		return errors.Reason("cwd must be absolute path").Err()
	}
	for _, vars := range [](map[string]string){c.ConfigVariables, c.PathVariables} {
		for k := range vars {
			if !isolate.IsValidVariable(k) {
				return fmt.Errorf("invalid key %s", k)
			}
		}
	}
	// Account for EXECUTABLE_SUFFIX.
	if len(c.ConfigVariables) != 0 || len(c.PathVariables) > 1 {
		os.Stderr.WriteString(
			"WARNING: -config-variables and -path-variables\n" +
				"         will be unsupported soon. Please contact the LUCI team.\n" +
				"         https://crbug.com/907880\n")
	}

	if c.Isolate == "" {
		return errors.Reason("-isolate must be specified").Err()
	}

	if !filepath.IsAbs(c.Isolate) {
		c.Isolate = filepath.Clean(filepath.Join(cwd, c.Isolate))
	}
	return nil
}

func elideNestedPaths(deps []string, pathSep string) []string {
	// For |deps| having a pattern like below:
	// "ab/"
	// "ab/cd/"
	// "ab/foo.txt"
	//
	// We need to elide the nested paths under "ab/" to make HardlinkRecursively
	// work. Without this step, all files have already been hard linked when
	// processing "ab/", so "ab/cd/" would lead to an error.
	sort.Strings(deps)
	prefixDir := ""
	var result []string
	for _, dep := range deps {
		if len(prefixDir) > 0 && strings.HasPrefix(dep, prefixDir) {
			continue
		}
		// |dep| can be either an unseen directory, or an individual file
		result = append(result, dep)
		prefixDir = ""
		if strings.HasSuffix(dep, pathSep) {
			// |dep| is a directory
			prefixDir = dep
		}
	}
	return result
}

func recreateTree(outDir string, rootDir string, deps []string) error {
	if err := filesystem.MakeDirs(outDir); err != nil {
		return errors.Annotate(err, "failed to create directory: %s", outDir).Err()
	}
	deps = elideNestedPaths(deps, string(os.PathSeparator))
	createdDirs := make(map[string]struct{})
	for _, dep := range deps {
		dst := filepath.Join(outDir, dep[len(rootDir):])
		dstDir := filepath.Dir(dst)
		if _, ok := createdDirs[dstDir]; !ok {
			if err := filesystem.MakeDirs(dstDir); err != nil {
				return errors.Annotate(err, "failed to call MakeDirs(%s)", dstDir).Err()
			}
			createdDirs[dstDir] = struct{}{}
		}

		err := filesystem.HardlinkRecursively(dep, dst)
		if err != nil {
			return errors.Annotate(err, "failed to call HardlinkRecursively(%s, %s)", dep, dst).Err()
		}
	}
	return nil
}

// archiveLogger reports stats to stderr.
type archiveLogger struct {
	start time.Time
	quiet bool
}

// LogSummary logs (to eventlog and stderr) a high-level summary of archive operations(s).
func (al *archiveLogger) LogSummary(ctx context.Context, hits, misses int, bytesHit, bytesMissed units.Size) {
	if !al.quiet {
		duration := time.Since(al.start)
		fmt.Fprintf(os.Stderr, "Hits    : %5d (%s)\n", hits, bytesHit)
		fmt.Fprintf(os.Stderr, "Misses  : %5d (%s)\n", misses, bytesMissed)
		fmt.Fprintf(os.Stderr, "Duration: %s\n", duration.Round(time.Millisecond))
	}
}

// Print acts like fmt.Printf, but may prepend a prefix to format, depending on the value of al.quiet.
func (al *archiveLogger) Printf(format string, a ...any) (n int, err error) {
	return al.Fprintf(os.Stdout, format, a...)
}

// Print acts like fmt.fprintf, but may prepend a prefix to format, depending on the value of al.quiet.
func (al *archiveLogger) Fprintf(w io.Writer, format string, a ...any) (n int, err error) {
	prefix := "\n"
	if al.quiet {
		prefix = ""
	}
	args := make([]any, 1+len(a))
	args[0] = prefix
	copy(args[1:], a)
	return fmt.Printf("%s"+format, args...)
}

func (r *baseCommandRun) uploadToCAS(ctx context.Context, dumpJSON string, authOpts auth.Options, fl *casclient.Flags, al *archiveLogger, opts ...*isolate.ArchiveOptions) ([]digest.Digest, error) {
	rootDgs, err := r.uploadToCASNew(ctx, authOpts, fl, al, opts...)
	if err != nil {
		return nil, err
	}

	if al != nil {
		for i, opt := range opts {
			if _, err := al.Printf("uploaded digest for %s: %s\n", opt.Isolate, rootDgs[i]); err != nil {
				return nil, err
			}
		}
	}

	if dumpJSON == "" {
		return rootDgs, nil
	}

	m := make(map[string]string, len(opts))
	for i, o := range opts {
		m[filesystem.GetFilenameNoExt(o.Isolate)] = rootDgs[i].String()
	}
	f, err := os.Create(dumpJSON)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return rootDgs, json.NewEncoder(f).Encode(m)
}

func (r *baseCommandRun) uploadToCASNew(ctx context.Context, authOpts auth.Options, fl *casclient.Flags, al *archiveLogger, opts ...*isolate.ArchiveOptions) ([]digest.Digest, error) {
	cl, err := r.newClient(ctx, fl.Addr, fl.Instance, authOpts, false)
	if err != nil {
		return nil, err
	}

	start := clock.Now(ctx)
	eg, ctx := errgroup.WithContext(ctx)

	// Prepare directories to upload.
	inputs := make([]*cas.UploadInput, len(opts))
	inputC := make(chan *cas.UploadInput)
	regexps := regexpCache{}
	eg.Go(func() error {
		defer close(inputC)
		for i, o := range opts {
			deps, path, err := isolate.ProcessIsolate(o)
			if err != nil {
				return err
			}

			in := &cas.UploadInput{
				Path:      path,
				Allowlist: make([]string, len(deps)),
			}
			for i, dep := range deps {
				if in.Allowlist[i], err = filepath.Rel(path, dep); err != nil {
					return errors.Annotate(err, "failed to compute relative path for %q", dep).Err()
				}
			}
			if o.IgnoredPathFilterRe != "" {
				if in.Exclude, err = regexps.Compile(o.IgnoredPathFilterRe); err != nil {
					return errors.Reason("invalid regexp %q: %s", o.IgnoredPathFilterRe, err).Err()
				}
			}

			inputs[i] = in
			select {
			case inputC <- in:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	// Upload the inputs.
	var uploadRes *cas.UploadResult
	eg.Go(func() (err error) {
		uploadRes, err = cl.Upload(ctx, cas.UploadOptions{
			PreserveSymlinks:      true,
			AllowDanglingSymlinks: true,
		}, inputC)
		if err != nil {
			// log for stacktrace.
			logging.Errorf(ctx, "failed to call Upload: %+v", err)
		}
		return errors.Annotate(err, "failed to call Upload").Err()
	})

	if err := eg.Wait(); err != nil {
		return nil, errors.Annotate(err, "failed to call Wait").Err()
	}

	// Collect digests.
	// Upload succeeded, so all digests are available by this time.
	rootDgs := make([]digest.Digest, len(inputs))
	for i, in := range inputs {
		if rootDgs[i], err = in.Digest("."); err != nil {
			// log for stacktrace.
			logging.Errorf(ctx, "failed to call Digest, %s: %+v", in.Path, err)
			return nil, errors.Annotate(err, "failed to retrieve digest for %q", in.Path).Err()
		}
	}

	// Log stats.
	st := &uploadRes.Stats
	logging.Infof(ctx, "finished upload for %d blobs (%d uploaded, %d bytes), took %s",
		st.CacheHits.Digests+st.CacheMisses.Digests,
		st.CacheMisses.Digests,
		st.CacheMisses.Bytes,
		clock.Since(ctx, start))
	if al != nil {
		al.LogSummary(ctx,
			int(st.CacheHits.Digests), int(st.CacheMisses.Digests),
			units.Size(st.CacheHits.Bytes), units.Size(st.CacheMisses.Bytes))
	}

	return rootDgs, nil
}

type regexpCache map[string]*regexp.Regexp

func (c regexpCache) Compile(expr string) (*regexp.Regexp, error) {
	if re, ok := c[expr]; ok {
		return re, nil
	}

	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}

	c[expr] = re
	return re, nil
}

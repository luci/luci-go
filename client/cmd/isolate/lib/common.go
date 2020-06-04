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
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/system/filesystem"
)

type commonFlags struct {
	subcommands.CommandRunBase
	defaultFlags common.Flags
}

func (c *commonFlags) Init() {
	c.defaultFlags.Init(&c.Flags)
}

func (c *commonFlags) Parse() error {
	return c.defaultFlags.Parse()
}

type commonServerFlags struct {
	commonFlags
	isolatedFlags isolatedclient.Flags
	authFlags     authcli.Flags

	parsedAuthOpts auth.Options
}

func (c *commonServerFlags) Init(authOpts auth.Options) {
	c.commonFlags.Init()
	c.isolatedFlags.Init(&c.Flags)
	c.authFlags.Register(&c.Flags, authOpts)
}

func (c *commonServerFlags) Parse() error {
	var err error
	if err = c.commonFlags.Parse(); err != nil {
		return err
	}
	if err = c.isolatedFlags.Parse(); err != nil {
		return err
	}
	c.parsedAuthOpts, err = c.authFlags.Options()
	return err
}

func (c *commonServerFlags) createAuthClient(ctx context.Context) (*http.Client, error) {
	// Don't enforce authentication by using OptionalLogin mode. This is needed
	// for IP whitelisted bots: they have NO credentials to send.
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, c.parsedAuthOpts).Client()
}

func (c *commonServerFlags) createIsolatedClient(authCl *http.Client) *isolatedclient.Client {
	userAgent := "isolate-go/" + IsolateVersion
	if ver, err := version.GetStartupVersion(); err == nil && ver.InstanceID != "" {
		userAgent += fmt.Sprintf(" (%s@%s)", ver.PackageName, ver.InstanceID)
	}
	return c.isolatedFlags.NewClient(isolatedclient.WithAuthClient(authCl), isolatedclient.WithUserAgent(userAgent))
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
	f.BoolVar(&c.AllowCommandAndRelativeCWD, "allow-command-and-relative-cwd", true, "This flag specifies whether client allows to use command and relative_cwd filed in isolate file. If you use this flag, make sure you report that to crbug.com/1069704 or contact to LUCI team.")
	f.BoolVar(&c.AllowMissingFileDir, "allow-missing-file-dir", false, "If this flag is true, invalid entries in the isolated file are only logged, but won't stop it from being processed.")
}

// RequiredIsolateFlags specifies which flags are required on the command line
// being parsed.
type RequiredIsolateFlags uint

const (
	// RequireIsolateFile means the -isolate flag is required.
	RequireIsolateFile RequiredIsolateFlags = 1 << iota
	// RequireIsolatedFile means the -isolated flag is required.
	RequireIsolatedFile
)

func (c *isolateFlags) Parse(cwd string, flags RequiredIsolateFlags) error {
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
		if flags&RequireIsolateFile != 0 {
			return errors.Reason("-isolate must be specified").Err()
		}
	} else {
		if !filepath.IsAbs(c.Isolate) {
			c.Isolate = filepath.Clean(filepath.Join(cwd, c.Isolate))
		}
	}

	if c.Isolated == "" {
		if flags&RequireIsolatedFile != 0 {
			return errors.Reason("-isolated must be specified").Err()
		}
	} else {
		if !filepath.IsAbs(c.Isolated) {
			c.Isolated = filepath.Clean(filepath.Join(cwd, c.Isolated))
		}
	}
	return nil
}

// loggingFlags configures eventlog logging.
type loggingFlags struct {
	EventlogEndpoint string
}

func (lf *loggingFlags) Init(f *flag.FlagSet) {
	f.StringVar(&lf.EventlogEndpoint, "eventlog-endpoint", "", `The URL destination for eventlogs. The special values "prod" or "test" may be used to target the standard prod or test urls respectively. An empty string disables eventlogging.`)
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

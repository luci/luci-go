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
	"errors"
	"flag"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/client/authcli"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/client/isolate"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging/gologger"
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

func (c *commonServerFlags) createAuthClient() (*http.Client, error) {
	// Don't enforce authentication by using OptionalLogin mode. This is needed
	// for IP whitelisted bots: they have NO credentials to send.
	ctx := gologger.StdConfig.Use(context.Background())
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, c.parsedAuthOpts).Client()
}

type isolateFlags struct {
	// TODO(tandrii): move ArchiveOptions from isolate pkg to here.
	isolate.ArchiveOptions
}

func (c *isolateFlags) Init(f *flag.FlagSet) {
	c.ArchiveOptions.Init()
	f.StringVar(&c.Isolate, "isolate", "", ".isolate file to load the dependency data from")
	f.StringVar(&c.Isolate, "i", "", "Alias for --isolate")
	f.StringVar(&c.Isolated, "isolated", "", ".isolated file to generate or read")
	f.StringVar(&c.Isolated, "s", "", "Alias for --isolated")
	f.Var(&c.Blacklist, "blacklist", "List of globs to use as blacklist filter when uploading directories")
	f.Var(&c.ConfigVariables, "config-variable", "Config variables are used to determine which conditions should be matched when loading a .isolate file, default: [].")
	f.Var(&c.PathVariables, "path-variable", "Path variables are used to replace file paths when loading a .isolate file, default: {}")
	f.Var(&c.ExtraVariables, "extra-variable", "Extraneous variables are replaced on the command entry and on paths in the .isolate file but are not considered relative paths.")
}

// RequiredIsolateFlags specifies which flags are required on the command line
// being parsed.
type RequiredIsolateFlags uint

const (
	// RequireIsolateFile means the --isolate flag is required.
	RequireIsolateFile RequiredIsolateFlags = 1 << iota
	// RequireIsolatedFile means the --isolated flag is required.
	RequireIsolatedFile
)

func (c *isolateFlags) Parse(cwd string, flags RequiredIsolateFlags) error {
	if !filepath.IsAbs(cwd) {
		return errors.New("cwd must be absolute path")
	}
	for _, vars := range [](map[string]string){c.ConfigVariables, c.ExtraVariables, c.PathVariables} {
		for k := range vars {
			if !isolate.IsValidVariable(k) {
				return fmt.Errorf("invalid key %s", k)
			}
		}
	}

	if c.Isolate == "" {
		if flags&RequireIsolateFile != 0 {
			return errors.New("-isolate must be specified")
		}
	} else {
		if !filepath.IsAbs(c.Isolate) {
			c.Isolate = filepath.Clean(filepath.Join(cwd, c.Isolate))
		}
	}

	if c.Isolated == "" {
		if flags&RequireIsolatedFile != 0 {
			return errors.New("-isolated must be specified")
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
	f.StringVar(&lf.EventlogEndpoint, "eventlog-endpoint", "", `The URL destination for eventlogs. The special values "prod" or "test" may be used to target the standard prod or test urls repspectively. An empty string disables eventlogging.`)
}

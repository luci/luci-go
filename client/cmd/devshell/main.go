// Copyright 2017 The LUCI Authors.
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

// Command devshell is a Devshell server.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/client/authcli"
	"go.chromium.org/luci/common/devshell"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/exitcode"

	"go.chromium.org/luci/hardcoded/chromeinfra"
)

const DevShellEnv = "DEVSHELL_CLIENT_PORT"

var (
	flags    authcli.Flags
	lifetime time.Duration
)

func init() {
	defaults := chromeinfra.DefaultAuthOptions()
	defaults.Scopes = []string{
		"https://www.googleapis.com/auth/devstorage.full_control",
		"https://www.googleapis.com/auth/devstorage.read_only",
		"https://www.googleapis.com/auth/devstorage.read_write",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
	}
	flags.RegisterScopesFlag = true
	flags.Register(flag.CommandLine, defaults)
	flag.DurationVar(
		&lifetime, "lifetime", time.Minute,
		"Minimum token lifetime. If existing token expired and refresh token or service account is not present, returns nothing.",
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: git-credential-luci command\n")
		flag.PrintDefaults()
	}
}

func setEnv(c *exec.Cmd, port int) {
	pfx := DevShellEnv + "="
	newVal := fmt.Sprintf("%s%d", pfx, port)
	if c.Env == nil {
		c.Env = os.Environ()
	}
	for i, l := range c.Env {
		if strings.HasPrefix(strings.ToUpper(l), pfx) {
			c.Env[i] = newVal
			return
		}
	}
	c.Env = append(c.Env, newVal)
}

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "specify a command to run")
		os.Exit(1)
	}

	opts, err := flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	bin := args[0]
	if filepath.Base(bin) == bin {
		path, err := exec.LookPath(bin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't find %q in PATH\n", bin)
			os.Exit(1)
		}
		bin = path
	}
	cmd := &exec.Cmd{
		Path:   bin,
		Args:   args,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	ctx := gologger.StdConfig.Use(context.Background())

	srv := &devshell.Server{
		Lifetime: lifetime,
		Options:  opts,
	}

	err = devshell.WithContext(ctx, srv, func(ctx context.Context) error {
		devshell := ctx.Value(&devshell.DevshellKey).(*devshell.DevshellContext)
		// Put the Devshell port into environment.
		setEnv(cmd, devshell.Port)

		fmt.Printf("Run devshell on port %d\n", devshell.Port)

		// Launch the process and wait for it to finish.
		logging.Debugf(ctx, "Running %q", cmd.Args)
		return cmd.Run()
	})

	// Return the subprocess exit code, if available.
	switch code, hasCode := exitcode.Get(err); {
	case hasCode:
		os.Exit(code)
	case err != nil:
		os.Exit(1)
	}
}

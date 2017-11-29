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

// Command gsutil-auth implements "gsutil -> LUCI auth" shim server.
//
// Use it as command wrapper:
// $ gsutil-auth gsutil ...
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/client/authcli"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/auth/gsutil"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/exitcode"

	"go.chromium.org/luci/hardcoded/chromeinfra"
)

var (
	flags    authcli.Flags
	lifetime time.Duration
)

func init() {
	defaults := chromeinfra.DefaultAuthOptions()
	defaults.Scopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
	}
	flags.RegisterScopesFlag = true
	flags.Register(flag.CommandLine, defaults)
	flag.DurationVar(
		&lifetime, "lifetime", time.Minute, "Minimum token lifetime.",
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: gsutil-auth command...\n")
		flag.PrintDefaults()
	}
}

func setBotoConfigEnv(c *exec.Cmd, botoCfg string) {
	pfx := "BOTO_CONFIG="
	if c.Env == nil {
		c.Env = os.Environ()
	}
	for i, l := range c.Env {
		if strings.HasPrefix(strings.ToUpper(l), pfx) {
			c.Env[i] = pfx + botoCfg
			return
		}
	}
	c.Env = append(c.Env, pfx+botoCfg)
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

	auth := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	source, err := auth.TokenSource()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get token source: %v\n", err)
		os.Exit(1)
	}

	// State dir is used to hold .boto and temporary credentials cache.
	stateDir, err := ioutil.TempDir("", "gsutil-auth")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create gsutil state dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(stateDir)

	srv := &gsutil.Server{
		Source:   source,
		StateDir: stateDir,
	}
	botoCfg, err := srv.Start(ctx)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to start the gsutil auth server")
		os.Exit(1)
	}
	defer srv.Stop(ctx) // close the server no matter what

	setBotoConfigEnv(cmd, botoCfg)

	// Return the subprocess exit code, if available.
	logging.Debugf(ctx, "Running %q", cmd.Args)
	switch code, hasCode := exitcode.Get(cmd.Run()); {
	case hasCode:
		os.Exit(code)
	case err != nil:
		logging.WithError(err).Errorf(ctx, "Command failed to start")
		os.Exit(1)
	}
}

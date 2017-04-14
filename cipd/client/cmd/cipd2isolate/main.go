// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package main contains a tool to upload cipd packages to an isolate server.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/cipd/client/cipd"
	cipdcommon "github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/cipd/client/cipd/ensure"
	cipdcli "github.com/luci/luci-go/cipd/client/cli"
	"github.com/luci/luci-go/cipd/version"
	"github.com/luci/luci-go/client/authcli"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/isolatedclient"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/hardcoded/chromeinfra"
)

func main() {
	mathrand.SeedRandomly()
	params := cipdcli.Parameters{
		DefaultAuthOptions: chromeinfra.DefaultAuthOptions(),
		ServiceURL:         chromeinfra.CIPDServiceURL,
	}
	os.Exit(subcommands.Run(GetApplication(params), os.Args[1:]))
}

// GetApplication returns cli.Application.
//
// It instantiates CIPD CLI application as a basis (for context, env vars, etc),
// and replaces all commands there.
func GetApplication(params cipdcli.Parameters) *cli.Application {
	app := *cipdcli.GetApplication(params)
	app.Name = "cipd2isolate"
	app.Title = "CIPD -> Isolate Uploader"
	app.Commands = []*subcommands.Command{
		subcommands.CmdHelp,
		version.SubcommandVersion,

		authcli.SubcommandInfo(params.DefaultAuthOptions, "auth-info", true),
		authcli.SubcommandLogin(params.DefaultAuthOptions, "auth-login", false),
		authcli.SubcommandLogout(params.DefaultAuthOptions, "auth-logout", false),

		cmdIsolate(params),
	}
	return &app
}

func cmdIsolate(params cipdcli.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "isolate [options]",
		ShortDesc: "isolates contents of bunch of CIPD packages",
		LongDesc: `Takes an 'ensure file' with description of some CIPD site root, ` +
			`and produces *.isolated with same content`,
		CommandRun: func() subcommands.CommandRun {
			c := &isolateRun{}

			c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to.")
			c.Flags.BoolVar(&c.verbose, "verbose", false, "Enable more logging.")
			c.authFlags.Register(&c.Flags, params.DefaultAuthOptions)

			c.Flags.StringVar(&c.cipdServiceURL, "cipd-service-url", params.ServiceURL,
				"CIPD Backend URL. If provided via an 'ensure file', the URL in the file takes precedence.")
			c.Flags.StringVar(&c.cipdCacheDir, "cipd-cache-dir", "",
				fmt.Sprintf("Directory for shared CIPD cache (can also be set by %s env var).", cipdcommon.CIPDCacheDir))
			c.Flags.StringVar(&c.ensureFile, "cipd-ensure-file", "",
				`An "ensure" file with packages to isolate. See syntax described here: `+
					`https://godoc.org/github.com/luci/luci-go/cipd/client/cipd/ensure.`+
					` Providing '-' will read from stdin.`)
			c.Flags.StringVar(&c.workDir, "work-dir", "./c2i_work", "A directory to keep temporary files in.")

			c.isolatedFlags.Init(&c.Flags)

			return c
		},
	}
}

type isolateRun struct {
	subcommands.CommandRunBase

	jsonOutput string
	verbose    bool
	authFlags  authcli.Flags

	cipdServiceURL string
	cipdCacheDir   string
	ensureFile     string
	workDir        string

	isolatedFlags isolatedclient.Flags
}

// ModifyContext implements cli.ContextModificator.
func (r *isolateRun) ModifyContext(ctx context.Context) context.Context {
	if r.verbose {
		ctx = logging.SetLevel(ctx, logging.Debug)
	}
	return ctx
}

// Run is the main entry point for 'cipd2isolate isolate'.
func (r *isolateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	ensureFile, err := r.parseEnsureFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse ensure file - %s\n", err)
		return 1
	}

	httpAuth, err := r.initHTTPAuth(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize authentication - %s\n", err)
		return 2
	}

	cipdClient, err := r.initCipdClient(ctx, httpAuth, ensureFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize CIPD client - %s\n", err)
		return 3
	}

	isolatedClient, err := r.initIsolatedClient(ctx, httpAuth)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize isolated client - %s\n", err)
		return 4
	}

	if err = isolateCipdPackages(ctx, ensureFile, cipdClient, isolatedClient); err != nil {
		return 5 // errors are logged already
	}
	return 0
}

func (r *isolateRun) parseEnsureFile() (*ensure.File, error) {
	if r.ensureFile == "" {
		return nil, fmt.Errorf("not provided, use -cipd-ensure-file flag to provide")
	}
	var err error
	var f io.ReadCloser
	if r.ensureFile == "-" {
		f = os.Stdin
	} else {
		if f, err = os.Open(r.ensureFile); err != nil {
			return nil, err
		}
	}
	defer f.Close()
	return ensure.ParseFile(f)
}

func (r *isolateRun) initHTTPAuth(ctx context.Context) (*http.Client, error) {
	authOpts, err := r.authFlags.Options()
	if err != nil {
		return nil, err
	}
	return auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
}

func (r *isolateRun) initCipdClient(ctx context.Context, httpAuth *http.Client, ensureFile *ensure.File) (cipd.Client, error) {
	// Prefer the ServiceURL from the file (if set), and log a warning if the user
	// provided one on the command line that doesn't match the one in the file.
	serviceURL := r.cipdServiceURL
	if ensureFile.ServiceURL != "" {
		if serviceURL != "" && serviceURL != ensureFile.ServiceURL {
			logging.Warningf(ctx, "serviceURL in ensure file != -cipd-service-url on CLI (%q v %q). Using %q from file.",
				ensureFile.ServiceURL, serviceURL, ensureFile.ServiceURL)
		}
		serviceURL = ensureFile.ServiceURL
	}

	cacheDir := r.cipdCacheDir
	if cacheDir == "" {
		var err error
		cacheDir, err = cipdcli.CacheDir(ctx)
		if err != nil {
			return nil, err
		}
	}

	return cipd.NewClient(cipd.ClientOptions{
		ServiceURL:          serviceURL,
		Root:                r.workDir,
		UserAgent:           cipdcli.UserAgent(ctx),
		CacheDir:            cacheDir,
		AuthenticatedClient: httpAuth,
		AnonymousClient:     http.DefaultClient,
	})
}

func (r *isolateRun) initIsolatedClient(ctx context.Context, httpAuth *http.Client) (*isolatedclient.Client, error) {
	if err := r.isolatedFlags.Parse(); err != nil {
		return nil, err
	}
	return isolatedclient.New(http.DefaultClient, httpAuth, r.isolatedFlags.ServerURL, r.isolatedFlags.Namespace, nil, nil), nil
}

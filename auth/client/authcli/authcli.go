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

// Package authcli implements authentication related flags parsing and CLI
// subcommands.
//
// It can be used from CLI tools that want customize authentication
// configuration from the command line.
//
// Minimal example of using flags parsing:
//
//
//	authFlags := authcli.Flags{}
//	defaults := ... // prepare default auth.Options
//	authFlags.Register(flag.CommandLine, defaults)
//	flag.Parse()
//	opts, err := authFlags.Options()
//	if err != nil {
//	  // handle error
//	}
//	authenticator := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
//	httpClient, err := authenticator.Client()
//	if err != nil {
//	  // handle error
//	}
//
//
// This assumes that either a service account credentials are used (passed via
// -service-account-json), or the user has previously ran "login" subcommand and
// their refresh token is already cached. In any case, there will be no
// interaction with the user (this is what auth.SilentLogin means): if there
// are no cached token, authenticator.Client will return auth.ErrLoginRequired.
//
// Interaction with the user happens only in "login" subcommand. This subcommand
// (as well as a bunch of other related commands) can be added to any
// subcommands.Application.
//
// While it will work with any subcommand.Application, it uses
// luci-go/common/cli.GetContext() to grab a context for logging, so callers
// should prefer using cli.Application for hosting auth subcommands and making
// the context. This ensures consistent logging style between all subcommands
// of a CLI application:
//
//
//	import (
//	  ...
//	  "go.chromium.org/luci/client/authcli"
//	  "go.chromium.org/luci/common/cli"
//	)
//
//	func GetApplication(defaultAuthOpts auth.Options) *cli.Application {
//	  return &cli.Application{
//	    Name:  "app_name",
//
//	    Context: func(ctx context.Context) context.Context {
//	      ... configure logging, etc. ...
//	      return ctx
//	    },
//
//	    Commands: []*subcommands.Command{
//	      authcli.SubcommandInfo(defaultAuthOpts, "auth-info", false),
//	      authcli.SubcommandLogin(defaultAuthOpts, "auth-login", false),
//	      authcli.SubcommandLogout(defaultAuthOpts, "auth-logout", false),
//	      ...
//	    },
//	  }
//	}
//
//	func main() {
//	  defaultAuthOpts := ...
//	  app := GetApplication(defaultAuthOpts)
//		os.Exit(subcommands.Run(app, nil))
//	}
package authcli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/authctx"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/lucictx"
)

// CommandParams specifies various parameters for a subcommand.
type CommandParams struct {
	Name     string // name of the subcommand.
	Advanced bool   // subcommands should treat this as an 'advanced' command

	AuthOptions auth.Options // default auth options.

	// ScopesFlag specifies if -scope flag must be registered.
	// AuthOptions.Scopes is used as a default value.
	// If it is empty, defaults to "https://www.googleapis.com/auth/userinfo.email".
	ScopesFlag bool
}

// Flags defines command line flags related to authentication.
type Flags struct {
	// RegisterScopesFlag tells Register to add -scopes flag.
	RegisterScopesFlag bool
	defaults           auth.Options
	serviceAccountJSON string
	scopes             string
}

// Register adds auth related flags to a FlagSet.
func (fl *Flags) Register(f *flag.FlagSet, defaults auth.Options) {
	fl.defaults = defaults
	f.StringVar(&fl.serviceAccountJSON, "service-account-json", fl.defaults.ServiceAccountJSONPath, "Path to JSON file with service account credentials to use.")
	if fl.RegisterScopesFlag {
		defaultScopes := strings.Join(defaults.Scopes, " ")
		if defaultScopes == "" {
			defaultScopes = auth.OAuthScopeEmail
		}
		f.StringVar(&fl.scopes, "scopes", defaultScopes, "space-separated OAuth 2.0 scopes")
	}
}

// Options return instance of auth.Options struct with values set accordingly to
// parsed command line flags.
func (fl *Flags) Options() (auth.Options, error) {
	opts := fl.defaults
	opts.ServiceAccountJSONPath = fl.serviceAccountJSON
	if fl.RegisterScopesFlag {
		opts.Scopes = strings.Split(fl.scopes, " ")
		sort.Strings(opts.Scopes)
	}
	return opts, nil
}

// Process exit codes for subcommands.
const (
	ExitCodeSuccess = iota
	ExitCodeNoValidToken
	ExitCodeInvalidInput
	ExitCodeInternalError
	ExitCodeBadLogin
)

type commandRunBase struct {
	subcommands.CommandRunBase
	flags  Flags
	params *CommandParams
}

func (c *commandRunBase) registerBaseFlags() {
	c.flags.RegisterScopesFlag = c.params.ScopesFlag
	c.flags.Register(&c.Flags, c.params.AuthOptions)
}

////////////////////////////////////////////////////////////////////////////////

// SubcommandLogin returns subcommands.Command that can be used to perform
// interactive login.
func SubcommandLogin(opts auth.Options, name string, advanced bool) *subcommands.Command {
	return SubcommandLoginWithParams(CommandParams{Name: name, Advanced: advanced, AuthOptions: opts})
}

// SubcommandLoginWithParams returns subcommands.Command that can be used to
// perform interactive login.
func SubcommandLoginWithParams(params CommandParams) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  params.Advanced,
		UsageLine: params.Name,
		ShortDesc: "performs interactive login flow",
		LongDesc:  "Performs interactive login flow and caches obtained credentials",
		CommandRun: func() subcommands.CommandRun {
			c := &loginRun{}
			c.params = &params
			c.registerBaseFlags()
			return c
		},
	}
}

type loginRun struct {
	commandRunBase
}

func (c *loginRun) Run(a subcommands.Application, _ []string, env subcommands.Env) int {
	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}
	ctx := cli.GetContext(a, c, env)
	authenticator := auth.NewAuthenticator(ctx, auth.InteractiveLogin, opts)
	if err := authenticator.Login(); err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %s\n", err.Error())
		return ExitCodeBadLogin
	}
	return checkToken(ctx, authenticator)
}

////////////////////////////////////////////////////////////////////////////////

// SubcommandLogout returns subcommands.Command that can be used to purge cached
// credentials.
func SubcommandLogout(opts auth.Options, name string, advanced bool) *subcommands.Command {
	return SubcommandLogoutWithParams(CommandParams{Name: name, Advanced: advanced, AuthOptions: opts})
}

// SubcommandLogoutWithParams returns subcommands.Command that can be used to purge cached
// credentials.
func SubcommandLogoutWithParams(params CommandParams) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  params.Advanced,
		UsageLine: params.Name,
		ShortDesc: "removes cached credentials",
		LongDesc:  "Removes cached credentials from the disk",
		CommandRun: func() subcommands.CommandRun {
			c := &logoutRun{}
			c.params = &params
			c.registerBaseFlags()
			return c
		},
	}
}

type logoutRun struct {
	commandRunBase
}

func (c *logoutRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}
	ctx := cli.GetContext(a, c, env)
	err = auth.NewAuthenticator(ctx, auth.SilentLogin, opts).PurgeCredentialsCache()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInternalError
	}
	return ExitCodeSuccess
}

////////////////////////////////////////////////////////////////////////////////

// SubcommandInfo returns subcommand.Command that can be used to print current
// cached credentials.
func SubcommandInfo(opts auth.Options, name string, advanced bool) *subcommands.Command {
	return SubcommandInfoWithParams(CommandParams{Name: name, Advanced: advanced, AuthOptions: opts})
}

// SubcommandInfoWithParams returns subcommand.Command that can be used to print
// current cached credentials.
func SubcommandInfoWithParams(params CommandParams) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  params.Advanced,
		UsageLine: params.Name,
		ShortDesc: "prints an email address associated with currently cached token",
		LongDesc:  "Prints an email address associated with currently cached token",
		CommandRun: func() subcommands.CommandRun {
			c := &infoRun{}
			c.params = &params
			c.registerBaseFlags()
			return c
		},
	}
}

type infoRun struct {
	commandRunBase
}

func (c *infoRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}
	ctx := cli.GetContext(a, c, env)
	authenticator := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	switch _, err := authenticator.Client(); {
	case err == auth.ErrLoginRequired:
		fmt.Fprintln(os.Stderr, "Not logged in")
		return ExitCodeNoValidToken
	case err != nil:
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInternalError
	}
	return checkToken(ctx, authenticator)
}

////////////////////////////////////////////////////////////////////////////////

// SubcommandToken returns subcommand.Command that can be used to print current
// access token.
func SubcommandToken(opts auth.Options, name string) *subcommands.Command {
	return SubcommandTokenWithParams(CommandParams{Name: name, AuthOptions: opts})
}

// SubcommandTokenWithParams returns subcommand.Command that can be used to
// print current access token.
func SubcommandTokenWithParams(params CommandParams) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  params.Advanced,
		UsageLine: params.Name,
		ShortDesc: "prints an access token",
		LongDesc:  "Generates an access token if requested and prints it.",
		CommandRun: func() subcommands.CommandRun {
			c := &tokenRun{}
			c.params = &params
			c.registerBaseFlags()
			c.Flags.DurationVar(
				&c.lifetime, "lifetime", time.Minute,
				"Minimum token lifetime. If existing token expired and refresh token or service account is not present, returns nothing.",
			)
			c.Flags.StringVar(
				&c.jsonOutput, "json-output", "",
				"Destination file to print token and expiration time in JSON. \"-\" for standard output.")
			return c
		},
	}
}

type tokenRun struct {
	commandRunBase
	lifetime   time.Duration
	jsonOutput string
}

func (c *tokenRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}
	if c.lifetime > 45*time.Minute {
		fmt.Fprintln(os.Stderr, "lifetime cannot exceed 45m")
		return ExitCodeInvalidInput
	}

	ctx := cli.GetContext(a, c, env)
	authenticator := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	token, err := authenticator.GetAccessToken(c.lifetime)
	if err != nil {
		if err == auth.ErrLoginRequired {
			fmt.Fprintln(os.Stderr, "Not logged in. Run 'luci-auth login'.")
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		return ExitCodeNoValidToken
	}
	if token.AccessToken == "" {
		return ExitCodeNoValidToken
	}

	if c.jsonOutput == "" {
		fmt.Println(token.AccessToken)
	} else {
		out := os.Stdout
		if c.jsonOutput != "-" {
			out, err = os.Create(c.jsonOutput)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return ExitCodeInvalidInput
			}
			defer out.Close()
		}
		data := struct {
			Token  string `json:"token"`
			Expiry int64  `json:"expiry"`
		}{token.AccessToken, token.Expiry.Unix()}
		if err = json.NewEncoder(out).Encode(data); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return ExitCodeInternalError
		}
	}
	return ExitCodeSuccess
}

////////////////////////////////////////////////////////////////////////////////

// SubcommandContext returns subcommand.Command that can be used to setup new
// LUCI authentication context for a process tree.
//
// This is an advanced command and shouldn't be usually embedded into binaries.
// It is primarily used by 'luci-auth' program. It exists to simplify
// development and debugging of programs that rely on LUCI authentication
// context.
func SubcommandContext(opts auth.Options, name string) *subcommands.Command {
	return SubcommandContextWithParams(CommandParams{Name: name, AuthOptions: opts})
}

// SubcommandContextWithParams returns subcommand.Command that can be used to
// setup new LUCI authentication context for a process tree.
func SubcommandContextWithParams(params CommandParams) *subcommands.Command {
	// By default request all scopes used by authctx.Context.
	params.AuthOptions.Scopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/firebase",
		"https://www.googleapis.com/auth/gerritcodereview",
		"https://www.googleapis.com/auth/userinfo.email",
	}
	return &subcommands.Command{
		Advanced:  params.Advanced,
		UsageLine: fmt.Sprintf("%s [flags] [--] <bin> [args]", params.Name),
		ShortDesc: "sets up new LUCI local auth context and launches a process in it",
		LongDesc:  "Starts local RPC auth server, prepares LUCI_CONTEXT, launches a process in this environment.",
		CommandRun: func() subcommands.CommandRun {
			c := &contextRun{}
			c.params = &params
			c.registerBaseFlags()
			c.Flags.StringVar(
				&c.actAs, "act-as-service-account", "",
				"Act as a given service account (caller must have iam.serviceAccountActor role).")
			c.Flags.BoolVar(&c.verbose, "verbose", false, "More logging")
			return c
		},
	}
}

type contextRun struct {
	commandRunBase

	actAs   string
	verbose bool
}

func (c *contextRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, c, env)

	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}
	opts.ActAsServiceAccount = c.actAs
	if c.verbose {
		ctx = logging.SetLevel(ctx, logging.Debug)
	}

	// 'args' specify a subcommand to run. Prepare *exec.Cmd.
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Specify a command to run:\n  auth-util context [flags] [--] <bin> [args]")
		return ExitCodeInvalidInput
	}
	bin := args[0]
	if filepath.Base(bin) == bin {
		resolved, err := exec.LookPath(bin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't find %q in PATH\n", bin)
			return ExitCodeInvalidInput
		}
		bin = resolved
	}
	cmd := &exec.Cmd{
		Path:   bin,
		Args:   args,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	// Create an authenticator for requested options to make sure we have required
	// refresh tokens (if any), asking the user to login if not.
	if opts.Method == auth.AutoSelectMethod {
		opts.Method = auth.SelectBestMethod(ctx, opts)
	}
	authenticator := auth.NewAuthenticator(ctx, auth.InteractiveLogin, opts)
	err = authenticator.CheckLoginRequired()
	if err == auth.ErrLoginRequired {
		fmt.Printf("Need to login first!\n\n")
		err = authenticator.Login()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeNoValidToken
	}

	// Now that there exists a cached token for requested options, we can launch
	// an auth context with all bells and whistles.
	authCtx := authctx.Context{
		ID:                 "luci-auth",
		Options:            opts,
		EnableGitAuth:      true,
		EnableDockerAuth:   true,
		EnableGCEEmulation: true,
		EnableFirebaseAuth: true,
	}
	if err = authCtx.Launch(ctx, ""); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInternalError
	}
	defer authCtx.Close(ctx) // logs errors inside

	// Adjust 'cmd' to run in the new modified environ.
	cmdEnv := environ.System()
	exported, err := lucictx.Export(authCtx.Export(ctx, cmdEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInternalError
	}
	defer exported.Close()
	exported.SetInEnviron(cmdEnv)

	// Launch the process and wait for it to finish. Return its exit code.
	logging.Debugf(ctx, "Running %q", cmd.Args)
	cmd.Env = cmdEnv.Sorted()
	switch code, hasCode := exitcode.Get(cmd.Run()); {
	case err == nil:
		return 0
	case hasCode:
		return code
	default:
		return ExitCodeInternalError
	}
}

////////////////////////////////////////////////////////////////////////////////

// checkToken prints information about the token carried by the authenticator.
//
// Prints errors to stderr and returns corresponding process exit code.
func checkToken(ctx context.Context, a *auth.Authenticator) int {
	// Grab the active access token.
	tok, err := a.GetAccessToken(time.Minute)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't grab an access token: %s\n", err)
		return ExitCodeNoValidToken
	}

	// Ask Google endpoint for details of the token.
	info, err := googleoauth.GetTokenInfo(ctx, googleoauth.TokenInfoParams{
		AccessToken: tok.AccessToken,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to call token info endpoint: %s\n", err)
		if err == googleoauth.ErrBadToken {
			return ExitCodeNoValidToken
		}
		return ExitCodeInternalError
	}

	// Email is set only if the token has userinfo.email scope.
	if info.Email != "" {
		fmt.Printf("Logged in as %s.\n", info.Email)
	} else {
		fmt.Printf("Logged in as uid %q.\n", info.Sub)
	}
	fmt.Printf("OAuth token details:\n")
	fmt.Printf("  Client ID: %s\n", info.Aud)
	fmt.Printf("  Scopes:\n")
	for _, scope := range strings.Split(info.Scope, " ") {
		fmt.Printf("    %s\n", scope)
	}

	return ExitCodeSuccess
}

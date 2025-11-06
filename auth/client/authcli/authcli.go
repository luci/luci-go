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
//	  os.Exit(subcommands.Run(app, nil))
//	}
package authcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/authctx"
	"go.chromium.org/luci/auth/credhelperpb"
	"go.chromium.org/luci/auth/internal"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/lucictx"
)

// CommandParams specifies various parameters for a subcommand.
type CommandParams struct {
	Name     string // name of the subcommand
	Advanced bool   // treat this as an 'advanced' subcommand

	AuthOptions auth.Options // default auth options

	// UseScopeFlags specifies whether scope-related flags must be registered.
	//
	// This is primarily used by `luci-auth` executable.
	//
	// UseScopeFlags is *not needed* for command line tools that call a fixed
	// number of backends. Just add all necessary scopes to AuthOptions.Scopes,
	// no need to expose a flag.
	UseScopeFlags bool

	// UseIDTokenFlags specifies whether to register flags related to ID tokens.
	//
	// This is primarily used by `luci-auth` executable.
	UseIDTokenFlags bool

	// UseCredentialHelperFlags specifies whether to register flags related to
	// external credential helpers.
	UseCredentialHelperFlags bool

	// UseADCFlags specifies whether to register flags related to Application
	// Default Credentials.
	UseADCFlags bool
}

// Flags defines command line flags related to authentication.
type Flags struct {
	defaults           auth.Options
	serviceAccountJSON string // value of -service-account-json

	hasScopeFlags bool   // true if registered -scopes (and related) flags
	scopes        string // value of -scopes
	scopesCloud   bool   // value of -scopes-cloud
	scopesContext bool   // value of -scopes-context
	scopesGerrit  bool   // value of -scopes-gerrit

	hasIDTokenFlags bool   // true if registered -use-id-token flag
	useIDToken      bool   // value of -use-id-token
	audience        string // value of -audience

	hasCredHelperFlags bool   // true if registered -credential-helper flag
	credHelper         string // value of -credential-helper

	hasADCFlags bool                 // true if registered -application-default-credentials
	adcPolicy   auth.GoogleADCPolicy // value of -application-default-credentials
}

// Register adds auth related flags to a FlagSet.
func (fl *Flags) Register(f *flag.FlagSet, defaults auth.Options) {
	fl.defaults = defaults
	if len(fl.defaults.Scopes) == 0 {
		fl.defaults.Scopes = scopes.DefaultScopeSet()
	}
	f.StringVar(&fl.serviceAccountJSON, "service-account-json", fl.defaults.ServiceAccountJSONPath,
		fmt.Sprintf("Path to JSON file with service account credentials to use. Or specify %q to use GCE's default service account.", auth.GCEServiceAccount))
}

// registerScopesFlags adds scope-related flags.
func (fl *Flags) registerScopesFlags(f *flag.FlagSet) {
	fl.hasScopeFlags = true
	f.StringVar(&fl.scopes, "scopes", strings.Join(fl.defaults.Scopes, " "),
		"Space-separated list of OAuth 2.0 scopes to use.")
	f.BoolVar(&fl.scopesCloud, "scopes-cloud", false,
		"When set, use scopes needed to call Google Cloud APIs. Overrides -scopes when present.")
	f.BoolVar(&fl.scopesCloud, "scopes-iam", false,
		"Alias for -scopes-cloud, for backward compatibility.")
	f.BoolVar(&fl.scopesContext, "scopes-context", false,
		"When set, use scopes needed to run `context` subcommand. Overrides -scopes when present.")
	f.BoolVar(&fl.scopesGerrit, "scopes-gerrit", false,
		"When set, use scopes needed to call Gerrit and Gitiles APIs. Overrides -scopes when present.")
}

// RegisterIDTokenFlags adds flags related to ID tokens.
func (fl *Flags) RegisterIDTokenFlags(f *flag.FlagSet) {
	fl.hasIDTokenFlags = true
	f.BoolVar(&fl.useIDToken, "use-id-token", false,
		"When set, use ID tokens instead of OAuth2 access tokens. Some backends may require them.")
	f.StringVar(&fl.audience, "audience", fl.defaults.Audience,
		"An audience to put into ID tokens. Ignored when not using ID tokens.")
}

// RegisterCredentialHelperFlags adds flags related to credential helpers.
func (fl *Flags) RegisterCredentialHelperFlags(f *flag.FlagSet) {
	fl.hasCredHelperFlags = true
	f.StringVar(&fl.credHelper, "credential-helper", "",
		"A specification of an external credential helper to use for minting authentication tokens. Experimental.")
}

// RegisterADCFlags adds flags related to Application Default Credentials.
func (fl *Flags) RegisterADCFlags(f *flag.FlagSet) {
	fl.hasADCFlags = true
	f.StringVar((*string)(&fl.adcPolicy), "application-default-credentials", string(auth.GoogleADCNever),
		fmt.Sprintf("When to use Application Default Credentials instead of LUCI user authentication: %q, %q or %q.",
			auth.GoogleADCAlways, auth.GoogleADCNever, auth.GoogleADCAllow),
	)
}

// Options returns auth.Options populated based on parsed command line flags.
func (fl *Flags) Options() (auth.Options, error) {
	opts := fl.defaults
	opts.ServiceAccountJSONPath = fl.serviceAccountJSON

	if fl.hasScopeFlags {
		sets := 0
		if fl.scopesCloud {
			sets++
			opts.Scopes = scopes.CloudScopeSet()
		}
		if fl.scopesContext {
			sets++
			opts.Scopes = scopes.ContextScopeSet()
		}
		if fl.scopesGerrit {
			sets++
			opts.Scopes = scopes.GerritScopeSet()
		}
		if sets > 1 {
			return auth.Options{}, fmt.Errorf("got multiple conflicting -scopes-* flag, at most one is allowed")
		}
		if sets == 0 {
			opts.Scopes = strings.Split(fl.scopes, " ")
			sort.Strings(opts.Scopes)
		}
	}

	if fl.hasIDTokenFlags {
		opts.UseIDTokens = fl.useIDToken
		opts.Audience = fl.audience
	}

	if fl.hasCredHelperFlags && fl.credHelper != "" {
		var cfg credhelperpb.Config
		if err := prototext.Unmarshal([]byte(fl.credHelper), &cfg); err != nil {
			return auth.Options{}, fmt.Errorf("bad -credential-helper value %q: %w", fl.credHelper, err)
		}
		if err := auth.CheckCredentialHelperConfig(&cfg); err != nil {
			return auth.Options{}, fmt.Errorf("bad -credential-helper value %q: %w", fl.credHelper, err)
		}
		opts.CredentialHelper = &cfg
	}

	if fl.hasADCFlags && fl.adcPolicy != "" {
		switch fl.adcPolicy {
		case auth.GoogleADCNever, auth.GoogleADCAlways, auth.GoogleADCAllow:
		default:
			return auth.Options{}, fmt.Errorf(
				"bad -application-default-credentials value %q: allowed values are %q, %q or %q",
				fl.adcPolicy, auth.GoogleADCNever, auth.GoogleADCAlways, auth.GoogleADCAllow)
		}
		opts.GoogleADCPolicy = fl.adcPolicy
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
	flags   Flags
	params  CommandParams
	verbose bool
}

func (c *commandRunBase) ModifyContext(ctx context.Context) context.Context {
	if c.verbose {
		ctx = logging.SetLevel(ctx, logging.Debug)
	}
	return ctx
}

func (c *commandRunBase) registerBaseFlags(params CommandParams) {
	c.params = params
	c.flags.Register(&c.Flags, c.params.AuthOptions)
	c.Flags.BoolVar(&c.verbose, "verbose", false, "More verbose logging.")
	if c.params.UseScopeFlags {
		c.flags.registerScopesFlags(&c.Flags)
	}
	if c.params.UseIDTokenFlags {
		c.flags.RegisterIDTokenFlags(&c.Flags)
	}
	if c.params.UseCredentialHelperFlags {
		c.flags.RegisterCredentialHelperFlags(&c.Flags)
	}
	if c.params.UseADCFlags {
		c.flags.RegisterADCFlags(&c.Flags)
	}
}

// askToLogin emits to stderr an instruction to login.
func (c *commandRunBase) askToLogin(opts auth.Options) {
	fmt.Fprintf(os.Stderr, "Not logged in.\n\nLogin by running:\n")
	fmt.Fprintf(os.Stderr, "   $ %s", opts.LoginCommandHint())
	fmt.Fprintf(os.Stderr, "\n")
}

////////////////////////////////////////////////////////////////////////////////

// SubcommandLogin returns subcommands.Command that can be used to perform
// interactive login.
func SubcommandLogin(opts auth.Options, name string, advanced bool) *subcommands.Command {
	return SubcommandLoginWithParams(CommandParams{
		Name:        name,
		Advanced:    advanced,
		AuthOptions: opts,
	})
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
			c.registerBaseFlags(params)
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
		fmt.Fprintf(os.Stderr, "Login failed: %s\n", err)
		return ExitCodeBadLogin
	}
	var tokenInfo tokenInfo
	if opts.UseIDTokens {
		tokenInfo, err = idTokenInfo(authenticator)
	} else {
		tokenInfo, err = oauthTokenInfo(ctx, authenticator)
	}
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		if errors.Is(err, errTokenInternal) {
			return ExitCodeInternalError
		}
		return ExitCodeNoValidToken
	}
	tokenInfo.PrintFormatted()
	return ExitCodeSuccess
}

////////////////////////////////////////////////////////////////////////////////

// SubcommandLogout returns subcommands.Command that can be used to purge cached
// credentials.
func SubcommandLogout(opts auth.Options, name string, advanced bool) *subcommands.Command {
	return SubcommandLogoutWithParams(CommandParams{
		Name:        name,
		Advanced:    advanced,
		AuthOptions: opts,
	})
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
			c.registerBaseFlags(params)
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
	return SubcommandInfoWithParams(CommandParams{
		Name:        name,
		Advanced:    advanced,
		AuthOptions: opts,
	})
}

// SubcommandInfoWithParams returns subcommand.Command that can be used to print
// current cached credentials.
func SubcommandInfoWithParams(params CommandParams) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  params.Advanced,
		UsageLine: params.Name,
		ShortDesc: "prints an email address associated with currently cached token",
		LongDesc:  "Prints or writes it to a JSON file an email address associated with currently cached token",
		CommandRun: func() subcommands.CommandRun {
			c := &infoRun{}
			c.registerBaseFlags(params)
			c.Flags.StringVar(
				&c.jsonOutput, "json-output", "",
				`Path to a JSON file to write the token into. Use "-" for standard output.`)
			return c
		},
	}
}

type infoRun struct {
	commandRunBase
	jsonOutput string
}

func (c *infoRun) Run(a subcommands.Application, args []string, env subcommands.Env) (exitCode int) {
	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}
	ctx := cli.GetContext(a, c, env)
	authenticator := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	switch _, err := authenticator.Client(); {
	case err == auth.ErrLoginRequired:
		fmt.Fprintln(os.Stderr, "Not logged in.")
		return ExitCodeNoValidToken
	case err != nil:
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInternalError
	}

	var tokenInfo tokenInfo
	if opts.UseIDTokens {
		tokenInfo, err = idTokenInfo(authenticator)
	} else {
		tokenInfo, err = oauthTokenInfo(ctx, authenticator)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, errTokenInternal) {
			return ExitCodeInternalError
		}
		return ExitCodeNoValidToken
	}

	exitCode = ExitCodeSuccess

	if c.jsonOutput == "" {
		// Print formatted info if -json-output is unset.
		tokenInfo.PrintFormatted()
	} else {
		// Print JSON to stdout by default.
		jsonWriter := os.Stdout

		// Otherwise, redirect JSON to the specified file.
		if c.jsonOutput != "-" {
			// If JSON is sent to a file, print formatted info to stdout.
			tokenInfo.PrintFormatted()

			jsonWriter, err = os.Create(c.jsonOutput)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return ExitCodeInvalidInput
			}
			defer func() {
				if err := jsonWriter.Close(); err != nil {
					fmt.Fprintln(os.Stderr, err)
					exitCode = ExitCodeInternalError
				}
			}()
		}

		// Write JSON to the destination determined above.
		if err = json.NewEncoder(jsonWriter).Encode(tokenInfo); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return ExitCodeInternalError
		}
	}
	return exitCode
}

////////////////////////////////////////////////////////////////////////////////

// SubcommandToken returns subcommand.Command that can be used to print current
// access token.
func SubcommandToken(opts auth.Options, name string) *subcommands.Command {
	return SubcommandTokenWithParams(CommandParams{
		Name:        name,
		AuthOptions: opts,
	})
}

// SubcommandTokenWithParams returns subcommand.Command that can be used to
// print current access token.
func SubcommandTokenWithParams(params CommandParams) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  params.Advanced,
		UsageLine: params.Name,
		ShortDesc: "prints an access or ID token",
		LongDesc:  "Refreshes the token (if necessary) and prints it or writes it to a JSON file.",
		CommandRun: func() subcommands.CommandRun {
			c := &tokenRun{}
			c.registerBaseFlags(params)
			c.Flags.DurationVar(
				&c.lifetime, "lifetime", time.Minute,
				"The returned token will live for at least that long. Depending on\n"+
					"what exact token provider is used internally, large values may not\n"+
					"work. Avoid using this parameter unless really necessary.\n"+
					"The maximum acceptable value is 30m.",
			)
			c.Flags.StringVar(
				&c.jsonOutput, "json-output", "",
				`Path to a JSON file to write the token into. Use "-" for standard output.`)
			c.Flags.StringVar(
				&c.jsonFormat, "json-format", "luci",
				"The format to be used when writing the token to a JSON file ('luci', 'reclient' or 'bazel').\n"+
					"The 'luci' format is {\"token\": \"...\", expiry: <unix_ts>}.\n"+
					"The 'reclient' format is similar, but uses the textual Unix date format for the expiry.\n"+
					"The 'bazel' format is defined in Bazel's credential helper design:\n"+
					"https://github.com/EngFlow/credential-helper-spec/blob/main/spec.md")
			return c
		},
	}
}

type tokenRun struct {
	commandRunBase
	lifetime   time.Duration
	jsonOutput string
	jsonFormat string
}

func (c *tokenRun) Run(a subcommands.Application, args []string, env subcommands.Env) (exitCode int) {
	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}
	if c.lifetime > 30*time.Minute {
		fmt.Fprintf(os.Stderr, "Requested -lifetime (%s) must not exceed 30m.\n", c.lifetime)
		return ExitCodeInvalidInput
	}
	if c.jsonFormat != "luci" && c.jsonFormat != "reclient" && c.jsonFormat != "bazel" {
		fmt.Fprintf(os.Stderr, "Unknown -json-format %q, must be 'luci', 'reclient' or 'bazel'.\n", c.jsonFormat)
		return ExitCodeInvalidInput
	}

	ctx := cli.GetContext(a, c, env)
	authenticator := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	token, err := authenticator.GetAccessToken(c.lifetime)
	if err != nil {
		if err == auth.ErrLoginRequired {
			c.askToLogin(opts)
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
			defer func() {
				if err := out.Close(); err != nil {
					fmt.Fprintln(os.Stderr, err)
					exitCode = ExitCodeInternalError
				}
			}()
		}
		var data any
		switch c.jsonFormat {
		case "luci":
			data = struct {
				Token  string `json:"token"`
				Expiry int64  `json:"expiry"`
			}{token.AccessToken, token.Expiry.Unix()}
		case "reclient":
			data = struct {
				Token  string `json:"token"`
				Expiry string `json:"expiry"`
			}{token.AccessToken, token.Expiry.UTC().Format(time.UnixDate)}
		case "bazel":
			data = struct {
				Headers map[string][]string `json:"headers"`
				Expires string              `json:"expires"`
			}{map[string][]string{"Authorization": []string{"Bearer " + token.AccessToken}}, token.Expiry.UTC().Format(time.RFC3339)}
		}
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
	return SubcommandContextWithParams(CommandParams{
		Name:        name,
		AuthOptions: opts,
	})
}

// SubcommandContextWithParams returns subcommand.Command that can be used to
// setup new LUCI authentication context for a process tree.
func SubcommandContextWithParams(params CommandParams) *subcommands.Command {
	params.AuthOptions.Scopes = scopes.ContextScopeSet()
	return &subcommands.Command{
		Advanced:  params.Advanced,
		UsageLine: fmt.Sprintf("%s [flags] [--] <bin> [args]", params.Name),
		ShortDesc: "sets up new LUCI local auth context and launches a process in it",
		LongDesc:  "Starts local RPC auth server, prepares LUCI_CONTEXT, launches a process in this environment.",
		CommandRun: func() subcommands.CommandRun {
			c := &contextRun{}
			c.registerBaseFlags(params)
			c.Flags.StringVar(
				&c.actAs, "act-as-service-account", "",
				"Act as a given service account (via Cloud IAM or via LUCI Token Server).")
			c.Flags.StringVar(
				&c.actViaRealm, "act-via-realm", params.AuthOptions.ActViaLUCIRealm,
				"When used together with -act-as-service-account enables account\n"+
					"impersonation through LUCI Token Server using LUCI Realms for ACLs.\n"+
					"Must have form `<project>:<realm>`. If unset, the impersonation will\n"+
					"be done through Cloud IAM instead bypassing LUCI.")
			c.Flags.StringVar(
				&c.tokenServerHost, "token-server-host", params.AuthOptions.TokenServerHost,
				"The LUCI Token Server hostname to use when using -act-via-realm.")
			c.Flags.BoolVar(
				&c.exposeSystemAccount, "expose-system-account", false,
				`Exposes non-default "system" LUCI logical account to emulate Swarming environment.`)
			c.Flags.BoolVar(
				&c.disableGitAuth, "disable-git-auth", false,
				"Toggles whether to attempt configuration of the git credentials environment\n"+
					"for the subprocess.")
			return c
		},
	}
}

type contextRun struct {
	commandRunBase

	actAs               string
	actViaRealm         string
	tokenServerHost     string
	exposeSystemAccount bool
	disableGitAuth      bool
}

func (c *contextRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, c, env)

	opts, err := c.flags.Options()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}
	opts.ActAsServiceAccount = c.actAs
	opts.ActViaLUCIRealm = c.actViaRealm
	opts.TokenServerHost = c.tokenServerHost

	// 'args' specify a subcommand to run.
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Specify a command to run:\n  %s context [flags] [--] <bin> [args]\n", os.Args[0])
		return ExitCodeInvalidInput
	}

	// Start watching for interrupts as soon as possible (in particular before
	// any heavy setup calls).
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, signals.Interrupts()...)
	defer func() {
		signal.Stop(interrupts)
		close(interrupts)
	}()

	// Create an authenticator for requested options to make sure we have required
	// refresh tokens (if any), asking the user to login if not.
	if opts.Method == auth.AutoSelectMethod {
		opts.Method = auth.SelectBestMethod(ctx, opts)
	}
	authenticator := auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
	if err = authenticator.CheckLoginRequired(); err != nil {
		if err == auth.ErrLoginRequired {
			c.askToLogin(opts)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		return ExitCodeNoValidToken
	}

	// Now that there exists a cached token for requested options, we can launch
	// an auth context with all bells and whistles. If you enable or disable
	// a feature here, make sure to adjust scopes.ContextScopeSet() as well.
	authCtx := authctx.Context{
		ID:                  "luci-auth",
		Options:             opts,
		ExposeSystemAccount: c.exposeSystemAccount,
		EnableGitAuth:       !c.disableGitAuth,
		EnableDockerAuth:    true,
		EnableGCEEmulation:  true,
		EnableFirebaseAuth:  true,
	}
	if err = authCtx.Launch(ctx, ""); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInternalError
	}
	defer authCtx.Close(ctx) // logs errors inside

	// Prepare a modified environ for the subcommand.
	cmdEnv := environ.System()
	exported, err := lucictx.Export(authCtx.Export(ctx, cmdEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInternalError
	}
	defer exported.Close()
	exported.SetInEnviron(cmdEnv)

	// Prepare the subcommand.
	logging.Debugf(ctx, "Running %q", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = cmdEnv.Sorted()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Rig it to die violently if the luci-auth unexpectedly dies. This works only
	// on Linux. See pdeath_linux.go and pdeath_notlinux.go.
	setPdeathsig(cmd)

	// Launch.
	if err = cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}

	// Forward interrupts to the child process. See terminate_windows.go and
	// terminate_notwindows.go.
	go func() {
		for sig := range interrupts {
			if err := terminateProcess(cmd.Process, sig); err != nil {
				logging.Errorf(ctx, "Failed to send %q to the child process: %s", sig, err)
			}
		}
	}()

	if err = cmd.Wait(); err == nil {
		return 0
	}
	if code, hasCode := exitcode.Get(err); hasCode {
		return code
	}
	return ExitCodeInternalError
}

////////////////////////////////////////////////////////////////////////////////

// errTokenInternal indicates a token could not be retrieved due to an internal error,
// rather than no token being available to retrieve.
var errTokenInternal = errors.New("internal error fetching token")

// tokenInfo interface defines how to print token information in a human-friendly manner.
type tokenInfo interface {
	PrintFormatted()
}

// IDTokenInfo contains details about an ID token.
type IDTokenInfo struct {
	Email    string `json:"email"`
	Issuer   string `json:"issuer"`
	Subject  string `json:"subject"`
	Audience string `json:"audience"`
}

// PrintFormatted prints the token in a human-friendly manner.
func (tokenInfo IDTokenInfo) PrintFormatted() {
	fmt.Printf("Logged in as %s.\n\n", tokenInfo.Email)
	fmt.Printf("ID token details:\n")
	fmt.Printf("  Issuer: %s\n", tokenInfo.Issuer)
	fmt.Printf("  Subject: %s\n", tokenInfo.Subject)
	fmt.Printf("  Audience: %s\n", tokenInfo.Audience)
}

// OAuthTokenInfo contains details about an OAuth access token.
type OAuthTokenInfo struct {
	Email    string   `json:"email,omitempty"`
	UID      string   `json:"uid,omitempty"`
	ClientID string   `json:"client_id"`
	Scopes   []string `json:"scopes"`
}

// PrintFormatted prints the token in a human-friendly manner.
func (tokenInfo OAuthTokenInfo) PrintFormatted() {
	if tokenInfo.Email != "" {
		fmt.Printf("Logged in as %s.\n\n", tokenInfo.Email)
	} else if tokenInfo.UID != "" {
		fmt.Printf("Logged in as uid %q.\n\n", tokenInfo.UID)
	}
	fmt.Printf("OAuth token details:\n")
	fmt.Printf("  Client ID: %s\n", tokenInfo.ClientID)
	fmt.Printf("  Scopes:\n")
	for _, scope := range tokenInfo.Scopes {
		fmt.Printf("    %s\n", scope)
	}
}

// idTokenInfo provides information about the token carried by the authenticator.
//
// Returns an IDTokenInfo struct.
func idTokenInfo(a *auth.Authenticator) (*IDTokenInfo, error) {
	// Grab the active token.
	tok, err := a.GetAccessToken(time.Minute)
	if err != nil {
		return nil, fmt.Errorf("can't grab an access token: %w", err)
	}

	// When using ID tokens, decode the claims and show some interesting ones.
	claims, err := internal.ParseIDTokenClaims(tok.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ID token: %w", err)
	}

	return &IDTokenInfo{
		Email:    claims.Email,
		Issuer:   claims.Iss,
		Subject:  claims.Sub,
		Audience: claims.Aud,
	}, nil
}

// oauthTokenInfo provides information about the token carried by the authenticator.
//
// Returns an OAuthTokenInfo struct.
func oauthTokenInfo(ctx context.Context, a *auth.Authenticator) (*OAuthTokenInfo, error) {
	// Grab the active token.
	tok, err := a.GetAccessToken(time.Minute)
	if err != nil {
		return nil, fmt.Errorf("can't grab an access token: %w", err)
	}

	// When using access tokens, ask the Google endpoint for details of the token.
	info, err := googleoauth.GetTokenInfo(ctx, googleoauth.TokenInfoParams{
		AccessToken: tok.AccessToken,
	})

	if errors.Is(err, googleoauth.ErrBadToken) {
		return nil, fmt.Errorf("failed to call token info endpoint: %w", err)
	} else if err != nil {
		return nil, fmt.Errorf("%w: %w", errTokenInternal, err)
	}

	oauthTokenInfo := &OAuthTokenInfo{
		ClientID: info.Aud,
		Scopes:   strings.Split(info.Scope, " "),
	}
	if info.Email != "" {
		oauthTokenInfo.Email = info.Email
	} else if info.Sub != "" {
		oauthTokenInfo.UID = info.Sub
	}
	return oauthTokenInfo, nil
}

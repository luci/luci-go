// Copyright 2014 The LUCI Authors.
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

package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"

	"go.chromium.org/luci/cipd/client/cipd"
)

// TODO(vadimsh): Add some tests.

// Parameters carry default configuration values for a CIPD CLI client.
type Parameters struct {
	// DefaultAuthOptions provide default values for authentication related
	// options (most notably SecretsDir: a directory with token cache).
	DefaultAuthOptions auth.Options

	// ServiceURL is a backend URL to use by default.
	ServiceURL string
}

////////////////////////////////////////////////////////////////////////////////
// Main.

// GetApplication returns cli.Application.
//
// It can be used directly by subcommands.Run(...), or nested into another
// application.
func GetApplication(params Parameters) *cli.Application {
	return &cli.Application{
		Name:  "cipd",
		Title: "Chrome Infra Package Deployer (" + cipd.UserAgent + ")",

		Context: func(ctx context.Context) context.Context {
			loggerConfig := gologger.LoggerConfig{
				Format: `[P%{pid} %{time:15:04:05.000} %{shortfile} %{level:.1s}] %{message}`,
				Out:    os.Stderr,
			}
			ctx, cancel := context.WithCancel(loggerConfig.Use(ctx))
			signals.HandleInterrupt(cancel)
			return ctx
		},

		EnvVars: map[string]subcommands.EnvVarDefinition{
			cipd.EnvConfigFile: {
				Advanced: true,
				ShortDesc: fmt.Sprintf(
					"Path to a config file to load instead of the default %q. If `-`, just ignore the default config file.",
					cipd.DefaultConfigFilePath(),
				),
			},
			cipd.EnvHTTPUserAgentPrefix: {
				Advanced:  true,
				ShortDesc: "Optional http User-Agent prefix.",
			},
			cipd.EnvCacheDir: {
				ShortDesc: "Directory with shared instance and tags cache " +
					"(-cache-dir, if given, takes precedence).",
			},
			cipd.EnvMaxThreads: {
				Advanced: true,
				ShortDesc: "Number of worker threads for extracting packages. " +
					"If 0 or negative, uses CPU count. (-max-threads, if given and not 0, takes precedence.)",
			},
			cipd.EnvParallelDownloads: {
				Advanced: true,
				ShortDesc: fmt.Sprintf("How many packages are allowed to be fetched concurrently. "+
					"If <=1, packages will be fetched sequentially. Default is %d.", cipd.DefaultParallelDownloads),
			},
			cipd.EnvAdmissionPlugin: {
				Advanced:  true,
				ShortDesc: "JSON-encoded list with a command line of a deployment admission plugin.",
			},
			cipd.EnvCIPDServiceURL: {
				Advanced:  true,
				ShortDesc: "Override CIPD service URL.",
			},
			cipd.EnvCIPDProxyURL: {
				Advanced:  true,
				ShortDesc: "If set, send requests here instead of the remote backend. Only unix://<path> is supported currently.",
			},
			envSimpleTerminalUI: {
				Advanced:  true,
				ShortDesc: "If set disables the fancy terminal UI with progress bars in favor of a simpler one that just logs to stderr.",
			},
		},

		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			versioncli.CmdVersion(cipd.UserAgent),

			// Authentication related commands.
			{}, // These are spacers so that the commands appear in groups.
			authcli.SubcommandInfoWithParams(authcli.CommandParams{
				Name:                     "auth-info",
				Advanced:                 true,
				AuthOptions:              params.DefaultAuthOptions,
				UseCredentialHelperFlags: true,
			}),
			authcli.SubcommandLogin(params.DefaultAuthOptions, "auth-login", false),
			authcli.SubcommandLogout(params.DefaultAuthOptions, "auth-logout", false),

			// High level read commands.
			{},
			cmdListPackages(params),
			cmdSearch(params),
			cmdResolve(params),
			cmdDescribe(params),
			cmdInstances(params),

			// High level remote write commands.
			{},
			cmdCreate(params),
			cmdAttach(params),
			cmdSetRef(params),
			cmdSetTag(params),
			cmdSetMetadata(params),

			// High level local write commands.
			{},
			cmdEnsure(params),
			cmdExport(params),
			cmdSelfUpdate(params),
			cmdSelfUpdateRoll(params),

			// Advanced ensure file operations.
			{Advanced: true},
			cmdEnsureFileVerify(params),
			cmdEnsureFileResolve(params),

			// User friendly subcommands that operates within a site root. Implemented
			// in friendly.go. These are advanced because they're half-baked.
			{Advanced: true},
			cmdInit(params),
			cmdInstall(params),
			cmdInstalled(params),

			// ACLs.
			{Advanced: true},
			cmdListACL(params),
			cmdEditACL(params),
			cmdCheckACL(params),

			// Low level pkg-* commands.
			{Advanced: true},
			cmdBuild(),
			cmdDeploy(),
			cmdFetch(params),
			cmdInspect(),
			cmdRegister(params),

			// Low level deployment-* commands.
			{Advanced: true},
			cmdCheckDeployment(params),
			cmdRepairDeployment(params),

			// Low level misc commands.
			{Advanced: true},
			cmdExpandPackageName(params),
			cmdPuppetCheckUpdates(params),

			// Utility commands.
			{Advanced: true},
			cmdProxy(params),
		},
	}
}

// Main runs the CIPD CLI.
func Main(params Parameters, args []string) int {
	return subcommands.Run(GetApplication(params), fixflagpos.FixSubcommands(args))
}

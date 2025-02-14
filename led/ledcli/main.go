// Copyright 2020 The LUCI Authors.
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

package ledcli

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/cli"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"

	"go.chromium.org/luci/led/job"
)

// userAgent is the HTTP user agent string for led.
var userAgent = "led 1.0.0"

func init() {
	ver, err := version.GetStartupVersion()
	if err != nil || ver.InstanceID == "" {
		return
	}
	userAgent += fmt.Sprintf(" (%s@%s)", ver.PackageName, ver.InstanceID)
}

func handleInterruption(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	signalC := make(chan os.Signal, 1)
	signal.Notify(signalC, os.Interrupt)
	go func() {
		interrupted := false
		for range signalC {
			if interrupted {
				os.Exit(1)
			}
			interrupted = true
			cancel()
		}
	}()
	return ctx
}

// Main executes the entire 'led' command line program, including argument
// parsing and exiting the binary.
//
// If you want to support 'kitchen' based swarming tasks, pass an implementation
// of `ks`. The only implementation of `ks` that matters is in the
// 'infra/tools/led2' package.
func Main(ks job.KitchenSupport) {
	if ks == nil {
		ks = job.NoKitchenSupport()
	}

	authOpts := chromeinfra.DefaultAuthOptions()
	authOpts.Scopes = []string{
		auth.OAuthScopeEmail,
		gerrit.OAuthScope,
	}
	defaults := cmdBaseOptions{
		authOpts:       authOpts,
		kitchenSupport: ks,
	}

	var application = cli.Application{
		Name: "led",
		Title: `'LUCI editor' - Multi-service LUCI job debugging tool.

Allows local modifications to LUCI jobs to be launched directly in swarming.
This is meant to aid in debugging and development for the interaction of
multiple LUCI services:
  * buildbucket
  * swarming
  * isolate
  * recipes
  * logdog
  * milo

This command is meant to be used multiple times in a pipeline. The flow is
generally:

  get | edit* | launch

Where the edit step(s) are optional. The output of the commands on stdout is
a JobDefinition JSON document, and the input to the commands is this same
JobDefinition JSON document. At any stage in the pipeline, you may, of course,
hand-edit the JobDefinition.

Example:
  led get-builder bucket_name:builder_name | \
    led edit-recipe-bundle -O recipe_engine=/local/recipe_engine > job.json
  # edit job.json by hand to inspect
  led edit-system -e CHROME_HEADLESS=1 < job.json | \
    led launch

This would pull the recipe job from the named swarming task, then isolate the
recipes from the current working directory (with an override for the
recipe_engine), and inject the isolate hash into the job, saving the result to
job.json. The user thens inspects job.json to look at the full set of flags and
features. After inspecting/editing the job, the user pipes it back through the
edit subcommand to set the swarming envvar $CHROME_HEADLESS=1, and then launches
the edited task back to swarming.

The source for led lives at:
  https://chromium.googlesource.com/infra/infra/+/HEAD/go/src/infra/tools/led

The spec (as it is) for JobDefinition is at:
  https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/led/job/job.proto
`,

		Context: func(ctx context.Context) context.Context {
			goLoggerCfg := gologger.LoggerConfig{Out: os.Stderr}
			goLoggerCfg.Format = "[%{level:.1s} %{time:2006-01-02 15:04:05}] %{message}"
			ctx = goLoggerCfg.Use(ctx)

			ctx = (&log.Config{Level: log.Info}).Set(ctx)
			return handleInterruption(ctx)
		},

		Commands: []*subcommands.Command{
			// commands to obtain JobDescriptions. These all begin with `get`.
			// TODO(iannucci): `get` to scrape from any URL
			getSwarmCmd(defaults),
			getBuildCmd(defaults),
			getBuilderCmd(defaults),

			// commands to edit JobDescriptions.
			editCmd(defaults),
			editSystemCmd(defaults),
			editRecipeBundleCmd(defaults),
			editGerritCLCmd(defaults),
			editCrCLCmd(defaults),
			editGitilesCommitCmd(defaults),
			editPayloadCmd(defaults),

			// commands to edit the raw isolated files.
			editIsolated(defaults),

			// commands to launch swarming tasks.
			launchCmd(defaults),
			// TODO(iannucci): launch-local to launch locally
			// TODO(iannucci): launch-buildbucket to launch on buildbucket

			{}, // spacer

			subcommands.CmdHelp,
			versioncli.CmdVersion(userAgent),

			{}, // spacer

			authcli.SubcommandLogin(defaults.authOpts, "auth-login", false),
			authcli.SubcommandLogout(defaults.authOpts, "auth-logout", false),
			authcli.SubcommandInfo(defaults.authOpts, "auth-info", false),
		},
	}

	os.Exit(subcommands.Run(&application, nil))
}

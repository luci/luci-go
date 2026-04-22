// Copyright 2026 The LUCI Authors.
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
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/digests"
	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// 'selfupdate' subcommand.

func cmdSelfUpdate(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "selfupdate -version <version> | -version-file <path>",
		ShortDesc: "updates the current CIPD client binary",
		LongDesc: "Does an in-place upgrade to the current CIPD binary.\n\n" +
			"Reads the version either from the command line (when using -version) or " +
			"from a file (when using -version-file). When using -version-file, also " +
			"loads special *.digests file (from <version-file>.digests path) with " +
			"pinned hashes of the client binary for all platforms. When selfupdating, " +
			"the client will verify the new downloaded binary has a hash specified in " +
			"the *.digests file.",
		CommandRun: func() subcommands.CommandRun {
			c := &selfupdateRun{}

			// By default, show a reduced number of logs unless something goes wrong.
			c.logConfig.Level = logging.Warning

			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "", "Version of the client to update to (incompatible with -version-file).")
			c.Flags.StringVar(&c.versionFile, "version-file", "",
				"Indicates the path to read the new version from (<version-file> itself) and "+
					"the path to the file with pinned hashes of the CIPD binary (<version-file>.digests file).")
			return c
		},
	}
}

type selfupdateRun struct {
	cipdSubcommand
	clientOptions

	version     string
	versionFile string
}

func (c *selfupdateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}

	ctx := cli.GetContext(a, c, env)

	switch {
	case c.version != "" && c.versionFile != "":
		return c.done(nil, makeCLIError("-version and -version-file are mutually exclusive, use only one"))
	case c.version == "" && c.versionFile == "":
		return c.done(nil, makeCLIError("either -version or -version-file are required"))
	}

	var version = c.version
	var digests *digests.ClientDigestsFile

	if version == "" { // using -version-file instead? load *.digests
		var err error
		version, err = loadClientVersion(c.versionFile)
		if err != nil {
			return c.done(nil, err)
		}
		digests, err = loadClientDigests(c.versionFile + digestsSfx)
		if err != nil {
			return c.done(nil, err)
		}
	}

	return c.done(func() (common.Pin, error) {
		exePath, err := os.Executable()
		if err != nil {
			return common.Pin{}, err
		}
		opts, err := c.clientOptions.toCIPDClientOpts(ctx, "")
		if err != nil {
			return common.Pin{}, err
		}
		return cipd.MaybeUpdateClient(ctx, opts, version, exePath, digests)
	}())
}

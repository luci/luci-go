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
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'export' subcommand.

func cmdExport(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "export [options]",
		ShortDesc: "writes packages to disk as a 'one-off'",
		LongDesc: `Writes packages to disk as a 'one-off'.

This writes packages to disk and discards any CIPD-related tracking data.  The
result is an installation of the specified packages on disk in a form you could
use to re-package them elsewhere (e.g. to create a tarball, zip, etc.), or for
some 'one time' use (where you don't need any of the guarantees provided by
"ensure").

In particular:
  * Export will blindly overwrite files in the target directory.
  * If a package removes a file(s) in some newer version, using "export"
    to write over an old version with the new version will not clean
    up the file(s).

Prepare an 'ensure file' as you would pass to the "ensure" subcommand.
This command implies "$OverrideInstallMode copy" in the ensure file.

    cipd export -root a/directory -ensure-file ensure_file

For the full syntax of the ensure file, see:

   https://go.chromium.org/luci/cipd/client/cipd/ensure
`,
		CommandRun: func() subcommands.CommandRun {
			c := &exportRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withMaxThreads)
			c.ensureFileOptions.registerFlags(&c.Flags, withEnsureOutFlag, withLegacyListFlag)

			return c
		},
	}
}

type exportRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *exportRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	lef, err := c.loadEnsureFile(ctx, ignoreVerifyPlatforms, parseVersionsFile)
	if err != nil {
		return c.done(nil, err)
	}
	lef.ef.OverrideInstallMode = pkg.InstallModeCopy

	pins, _, err := ensurePackages(ctx, lef, c.ensureFileOut, false, c.clientOptions)
	if err != nil {
		return c.done(pins, err)
	}

	logging.Infof(ctx, "Removing cipd metadata")
	fsystem := fs.NewFileSystem(c.rootDir, "")
	ssd, err := fsystem.RootRelToAbs(fs.SiteServiceDir)
	if err != nil {
		err = cipderr.IO.Apply(errors.Fmt("unable to resolve service dir: %w", err))
	} else if err = fsystem.EnsureDirectoryGone(ctx, ssd); err != nil {
		err = cipderr.IO.Apply(errors.Fmt("unable to purge service dir: %w", err))
	}
	return c.done(pins, err)
}

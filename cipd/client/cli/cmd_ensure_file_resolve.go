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
	"fmt"
	"path/filepath"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'ensure-file-resolve' subcommand.

func cmdEnsureFileResolve(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ensure-file-resolve [options]",
		ShortDesc: "resolves versions of all packages and writes them into $ResolvedVersions file",
		LongDesc: "Resolves versions of all packages for all verified platforms in the \"ensure\" file.\n\n" +
			`Writes them to a file specified by $ResolvedVersions directive in the ensure file, ` +
			`to be used for version resolution during "cipd ensure ..." instead of calling the backend.`,
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			c := &ensureFileResolveRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.ensureFileOptions.registerFlags(&c.Flags, withoutEnsureOutFlag, withoutLegacyListFlag)
			return c
		},
	}
}

type ensureFileResolveRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *ensureFileResolveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	ef, err := c.loadEnsureFile(ctx, &c.clientOptions, requireVerifyPlatforms, ignoreVersionsFile)
	switch {
	case err != nil:
		return c.done(nil, err)
	case ef.ResolvedVersions == "":
		logging.Errorf(ctx,
			"The ensure file doesn't have $ResolvedVersion directive that specifies "+
				"where to put the resolved package versions, so it can't be resolved.")
		return c.done(nil, cipderr.BadArgument.Apply(errors.New("no resolved versions file configured")))
	}

	pinMap, versions, err := resolveEnsureFile(ctx, ef, c.clientOptions)
	if err != nil {
		return c.doneWithPinMap(pinMap, err)
	}

	if err := saveVersionsFile(ef.ResolvedVersions, versions); err != nil {
		return c.done(nil, err)
	}

	fmt.Printf("The resolved versions have been written to %s.\n\n", filepath.Base(ef.ResolvedVersions))
	return c.doneWithPinMap(pinMap, nil)
}

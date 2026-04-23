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
	"path/filepath"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'ensure-file-verify' subcommand.

func cmdEnsureFileVerify(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ensure-file-verify [options]",
		ShortDesc: "verifies packages in a manifest exist for all platforms",
		LongDesc: "Verifies that the packages in the \"ensure\" file exist for all platforms.\n\n" +
			"Additionally if the ensure file uses $ResolvedVersions directive, checks that " +
			"all versions there are up-to-date. Returns non-zero if some version can't be " +
			"resolved or $ResolvedVersions file is outdated.",
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			c := &ensureFileVerifyRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.ensureFileOptions.registerFlags(&c.cipdSubcommand, singleEnsureFile, withoutEnsureOutFlag, withoutLegacyListFlag)
			return c
		},
	}
}

type ensureFileVerifyRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *ensureFileVerifyRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	lef, err := c.loadEnsureFile(ctx, requireVerifyPlatforms, ignoreVersionsFile)
	if err != nil {
		return c.done(nil, err)
	}

	// Resolving all versions in the ensure file also naturally verifies all
	// versions exist.
	pinMap, versions, err := resolveEnsureFile(ctx, lef, c.clientOptions)
	if err != nil || lef.ef.ResolvedVersions == "" {
		return c.doneWithPinMap(pinMap, err)
	}

	// Verify $ResolvedVersions file is up-to-date too.
	switch existing, err := loadVersionsFile(lef.ef.ResolvedVersions, lef.path); {
	case err != nil:
		return c.done(nil, err)
	case !existing.Equal(versions):
		return c.done(nil,
			cipderr.Stale.Apply(errors.Fmt("the resolved versions file %s is stale, "+
				"use 'cipd ensure-file-resolve -ensure-file %q' to update it",
				filepath.Base(lef.ef.ResolvedVersions), lef.path)))
	default:
		return c.doneWithPinMap(pinMap, err)
	}
}

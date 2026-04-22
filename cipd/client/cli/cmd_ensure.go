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
	"bytes"
	"context"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'ensure' subcommand.

func cmdEnsure(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ensure [options]",
		ShortDesc: "installs, removes and updates packages in one go",
		LongDesc: `Installs, removes and updates packages in one go.

Prepare an 'ensure file' by listing packages and their versions, each on their
own line, e.g.:

    some/package/name/${platform}  version:1.2.3
    other/package                  some_ref

Then use the ensure command to read this ensure file and 'ensure' that a given
folder has the packages at the versions specified:

    cipd ensure -root a/directory -ensure-file ensure_file

For the full syntax of the ensure file, see:

   https://go.chromium.org/luci/cipd/client/cipd/ensure
`,
		CommandRun: func() subcommands.CommandRun {
			c := &ensureRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withMaxThreads)
			c.ensureFileOptions.registerFlags(&c.Flags, withEnsureOutFlag, withLegacyListFlag)
			return c
		},
	}
}

type ensureRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *ensureRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	lef, err := c.loadEnsureFile(ctx, ignoreVerifyPlatforms, parseVersionsFile)
	if err != nil {
		return c.done(nil, err)
	}

	pins, _, err := ensurePackages(ctx, lef, c.ensureFileOut, false, c.clientOptions)
	return c.done(pins, err)
}

func ensurePackages(ctx context.Context, lef *loadedEnsureFile, ensureFileOut string, dryRun bool, clientOpts clientOptions) (common.PinSliceBySubdir, cipd.ActionMap, error) {
	client, err := clientOpts.makeCIPDClient(ctx, lef.ef.ServiceURL, lef.vf)
	if err != nil {
		return nil, nil, err
	}
	defer client.Close(ctx)

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	resolver := cipd.Resolver{Client: client}
	resolved, err := resolver.Resolve(ctx, lef.ef, template.DefaultExpander())
	if err != nil {
		return nil, nil, err
	}

	actions, err := client.EnsurePackages(ctx, resolved.PackagesBySubdir, &cipd.EnsureOptions{
		Paranoia:            resolved.ParanoidMode,
		DryRun:              dryRun,
		OverrideInstallMode: resolved.OverrideInstallMode,
	})
	if err != nil {
		return nil, actions, err
	}

	if ensureFileOut != "" {
		buf := bytes.Buffer{}
		resolved.ServiceURL = clientOpts.resolvedServiceURL(ctx, lef.ef.ServiceURL)
		resolved.ParanoidMode = ""
		if err = resolved.Serialize(&buf); err == nil {
			err = os.WriteFile(ensureFileOut, buf.Bytes(), 0666)
			if err != nil {
				err = cipderr.IO.Apply(errors.Fmt("writing resolved ensure file: %w", err))
			}
		}
	}

	return resolved.PackagesBySubdir, actions, err
}

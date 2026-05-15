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
	"maps"
	"path/filepath"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'cache-prepare' subcommand.

func cmdCachePrepare(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "cache-prepare [-trim] -ensure-file ef [-ensure-file ef]... path/to/cache",
		ShortDesc: "resolves versions of all packages in all ensure files and fetches them into a cache dir",
		LongDesc: `Resolves all ensure files into a version cache and fetches all instances into the ` +
			`instance cache, writing them to the target path. This can be used to prepare a cache bundle ` +
			`for use with $CIPD_OFFLINE_CACHE.`,
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			c := &cachePrepareRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.ensureFileOptions.registerFlags(&c.cipdSubcommand, oneOrMoreEnsureFile, withoutEnsureOutFlag, withoutLegacyListFlag)
			c.Flags.BoolVar(&c.trim, "trim", false, "If set, remove all instances and versions from the cache except for the given ensure files.")
			return c
		},
	}
}

type cachePrepareRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions

	trim bool
}

func (c *cachePrepareRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	targetPath, err := filepath.Abs(args[0])
	if err != nil {
		return c.done(nil, cipderr.BadArgument.Apply(errors.Fmt("output cache path: %w", err)))
	}

	opts, err := c.clientOptions.toCIPDClientOpts(ctx, "", nil)
	if err != nil {
		return c.done(nil, err)
	}

	versions := cipd.PackageVersionSet{}
	for lef, err := range c.allEnsureFiles(ctx, ignoreVerifyPlatforms, parseVersionsFile) {
		if err != nil {
			return c.done(nil, errors.Fmt("parsing %q: %w", lef.path, err))
		}
		vmap, err := cipd.OfflineResolve(opts, lef.ef, lef.vf)
		if err != nil {
			return c.done(nil, errors.Fmt("processing %q: %w", lef.path, err))
		}
		maps.Insert(versions, maps.All(vmap))
	}

	return c.done(nil, cipd.CachePrepare(ctx, opts, targetPath, c.trim, versions))
}

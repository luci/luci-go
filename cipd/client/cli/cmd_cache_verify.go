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

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'cache-verify' subcommand.

func cmdCacheVerify(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "cache-verify [-clean] path/to/cache",
		ShortDesc: "checks integrity of cache",
		LongDesc: ("Checks hashes of all instances, and checks all cached versions against the server.\n" +
			"\n" +
			"Cached tags will be checked against the server to see that they have the same\n" +
			"instance ID as the cached value.\n" +
			"\n" +
			"Cached refs will be checked that the server still has the referenced InstanceID,\n" +
			"not that the ref currently has the same value\n" +
			"\n" +
			"If `-clean` is given, then bad instances, tags and refs will be removed."),
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			c := &cacheVerifyRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.BoolVar(&c.clean, "clean", false, "remove bad instances/versions.")
			return c
		},
	}
}

type cacheVerifyRun struct {
	cipdSubcommand
	clientOptions

	clean bool
}

func (c *cacheVerifyRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	targetPath, err := filepath.Abs(args[0])
	if err != nil {
		return c.done(nil, cipderr.BadArgument.Apply(errors.Fmt("target cache path: %w", err)))
	}

	opts, err := c.clientOptions.toCIPDClientOpts(ctx, "", nil)
	if err != nil {
		return c.done(nil, err)
	}

	return c.done(nil, cipd.CacheVerify(ctx, opts, targetPath, c.clean))
}

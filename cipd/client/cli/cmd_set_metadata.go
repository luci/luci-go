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

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// 'set-metadata' subcommand.

func cmdSetMetadata(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "set-metadata <package or package prefix> -metadata key:value -metadata-from-file key:path [options]",
		ShortDesc: "attaches metadata to an instance",
		LongDesc:  "Attaches metadata to an instance",
		CommandRun: func() subcommands.CommandRun {
			c := &setMetadataRun{}
			c.registerBaseFlags()
			c.metadataOptions.registerFlags(&c.Flags)
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>",
				"Package version to resolve. Could also be a tag or a ref.")
			return c
		},
	}
}

type setMetadataRun struct {
	cipdSubcommand
	metadataOptions
	clientOptions

	version string
}

func (c *setMetadataRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}

	ctx := cli.GetContext(a, c, env)

	md, err := c.metadataOptions.load(ctx)
	if err == nil && len(md) == 0 {
		err = makeCLIError("at least one -metadata or -metadata-from-file must be provided")
	}
	if err != nil {
		return c.done(nil, err)
	}

	pkgPrefix, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	return c.doneWithPins(visitPins(ctx, &visitPinsArgs{
		clientOptions: c.clientOptions,
		packagePrefix: pkgPrefix,
		version:       c.version,
		updatePin: func(client cipd.Client, pin common.Pin) error {
			return client.AttachMetadataWhenReady(ctx, pin, md)
		},
	}))
}

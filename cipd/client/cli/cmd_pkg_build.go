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
	"context"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'pkg-build' subcommand.

func cmdBuild() *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-build [options]",
		ShortDesc: "builds a package instance file",
		LongDesc:  "Builds a package instance producing *.cipd file.",
		CommandRun: func() subcommands.CommandRun {
			c := &buildRun{}
			c.registerBaseFlags()
			c.inputOptions.registerFlags(&c.Flags)
			c.hashOptions.registerFlags(&c.Flags)
			c.Flags.StringVar(&c.outputFile, "out", "<path>", "Path to a file to write the final package to.")
			return c
		},
	}
}

type buildRun struct {
	cipdSubcommand
	inputOptions
	hashOptions

	outputFile string
}

func (c *buildRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	_, err := buildInstanceFile(ctx, c.outputFile, c.inputOptions, c.hashAlgo())
	if err != nil {
		return c.done(nil, err)
	}
	return c.done(inspectInstanceFile(ctx, c.outputFile, c.hashAlgo(), false))
}

func buildInstanceFile(ctx context.Context, instanceFile string, inputOpts inputOptions, algo caspb.HashAlgo) (common.Pin, error) {
	// Read the list of files to add to the package.
	buildOpts, err := inputOpts.prepareInput()
	if err != nil {
		return common.Pin{}, err
	}

	// Prepare the destination, update build options with io.Writer to it.
	out, err := os.OpenFile(instanceFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("opening instance file for writing: %w", err))
	}
	buildOpts.Output = out
	buildOpts.HashAlgo = algo

	// Build the package.
	pin, err := builder.BuildInstance(ctx, buildOpts)
	if err != nil {
		out.Close()
		os.Remove(instanceFile)
		return common.Pin{}, err
	}

	// Make sure it is flushed properly by ensuring Close succeeds.
	if err := out.Close(); err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("flushing built instance file: %w", err))
	}

	return pin, nil
}

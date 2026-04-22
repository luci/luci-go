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

	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'pkg-fetch' subcommand.

func cmdFetch(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-fetch <package> [options]",
		ShortDesc: "fetches a package instance file from the repository",
		LongDesc:  "Fetches a package instance file from the repository.",
		CommandRun: func() subcommands.CommandRun {
			c := &fetchRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>", "Package version to fetch.")
			c.Flags.StringVar(&c.outputPath, "out", "<path>", "Path to a file to write fetch to.")
			return c
		},
	}
}

type fetchRun struct {
	cipdSubcommand
	clientOptions

	version    string
	outputPath string
}

func (c *fetchRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	pkg, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.done(fetchInstanceFile(ctx, pkg, c.version, c.outputPath, c.clientOptions))
}

func fetchInstanceFile(ctx context.Context, packageName, version, instanceFile string, clientOpts clientOptions) (pin common.Pin, err error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return common.Pin{}, err
	}
	defer client.Close(ctx)

	defer func() {
		cipderr.AttachDetails(&err, cipderr.Details{
			Package: packageName,
			Version: version,
		})
	}()

	pin, err = client.ResolveVersion(ctx, packageName, version)
	if err != nil {
		return common.Pin{}, err
	}

	out, err := os.OpenFile(instanceFile, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("opening the instance file for writing: %w", err))
	}
	ok := false
	defer func() {
		if !ok {
			out.Close()
			os.Remove(instanceFile)
		}
	}()

	err = client.FetchInstanceTo(ctx, pin, out)
	if err != nil {
		return common.Pin{}, err
	}

	if err := out.Close(); err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("flushing fetched instance file: %w", err))
	}
	ok = true

	// Print information about the instance. 'FetchInstanceTo' already verified
	// the hash.
	inst, err := reader.OpenInstanceFile(ctx, instanceFile, reader.OpenInstanceOpts{
		VerificationMode: reader.SkipHashVerification,
		InstanceID:       pin.InstanceID,
	})
	if err != nil {
		os.Remove(instanceFile)
		return common.Pin{}, err
	}
	defer inst.Close(ctx, false)
	inspectInstance(ctx, inst, false)
	return inst.Pin(), nil
}

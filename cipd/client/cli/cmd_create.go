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

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'create' subcommand.

func cmdCreate(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "create [options]",
		ShortDesc: "builds and uploads a package instance file",
		LongDesc:  "Builds and uploads a package instance file.",
		CommandRun: func() subcommands.CommandRun {
			c := &createRun{}
			c.registerBaseFlags()
			c.Opts.inputOptions.registerFlags(&c.Flags)
			c.Opts.refsOptions.registerFlags(&c.Flags)
			c.Opts.tagsOptions.registerFlags(&c.Flags)
			c.Opts.metadataOptions.registerFlags(&c.Flags)
			c.Opts.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Opts.uploadOptions.registerFlags(&c.Flags)
			c.Opts.hashOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type createOpts struct {
	inputOptions
	refsOptions
	tagsOptions
	metadataOptions
	clientOptions
	uploadOptions
	hashOptions
}

type createRun struct {
	cipdSubcommand

	Opts createOpts
}

func (c *createRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(buildAndUploadInstance(ctx, &c.Opts))
}

func buildAndUploadInstance(ctx context.Context, opts *createOpts) (common.Pin, error) {
	f, err := os.CreateTemp("", "cipd_pkg")
	if err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("creating temp instance file: %w", err))
	}
	defer func() {
		// Note: we don't care about errors here since this file is used only as
		// a temporary buffer between buildInstanceFile and registerInstanceFile.
		// These functions check that everything was uploaded correctly using
		// hashes.
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()
	pin, err := buildInstanceFile(ctx, f.Name(), opts.inputOptions, opts.hashAlgo())
	if err != nil {
		return common.Pin{}, err
	}
	return registerInstanceFile(ctx, f.Name(), &pin, &registerOpts{
		refsOptions:     opts.refsOptions,
		tagsOptions:     opts.tagsOptions,
		metadataOptions: opts.metadataOptions,
		clientOptions:   opts.clientOptions,
		uploadOptions:   opts.uploadOptions,
		hashOptions:     opts.hashOptions,
	})
}

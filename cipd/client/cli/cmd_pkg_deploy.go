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

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/client/cipd/deployer"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// 'pkg-deploy' subcommand.

func cmdDeploy() *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-deploy <package instance file> [options]",
		ShortDesc: "deploys a package instance file",
		LongDesc:  "Deploys a *.cipd package instance into a site root.",
		CommandRun: func() subcommands.CommandRun {
			c := &deployRun{}
			c.registerBaseFlags()
			c.hashOptions.registerFlags(&c.Flags)
			c.maxThreadsOption.registerFlags(&c.Flags)
			c.Flags.StringVar(&c.rootDir, "root", "<path>", "Path to an installation site root directory.")
			c.Flags.Var(&c.overrideInstallMode, "install-mode",
				"Deploy using this mode instead, even if the instance manifest specifies otherwise.")
			return c
		},
	}
}

type deployRun struct {
	cipdSubcommand
	hashOptions
	maxThreadsOption

	rootDir             string
	overrideInstallMode pkg.InstallMode
}

func (c *deployRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	maxThreads, err := c.loadMaxThreads(ctx)
	if err != nil {
		return c.done(nil, err)
	}
	return c.done(deployInstanceFile(ctx, c.rootDir, args[0], c.hashAlgo(), maxThreads, c.overrideInstallMode))
}

func deployInstanceFile(ctx context.Context, root, instanceFile string, hashAlgo caspb.HashAlgo, maxThreads int, overrideInstallMode pkg.InstallMode) (common.Pin, error) {
	inst, err := reader.OpenInstanceFile(ctx, instanceFile, reader.OpenInstanceOpts{
		VerificationMode: reader.CalculateHash,
		HashAlgo:         hashAlgo,
	})
	if err != nil {
		return common.Pin{}, err
	}
	defer inst.Close(ctx, false)

	inspectInstance(ctx, inst, false)

	d := deployer.New(root)
	defer d.FS().CleanupTrash(ctx)

	// TODO(iannucci): add subdir arg to deployRun

	return d.DeployInstance(ctx, "", inst, overrideInstallMode, maxThreads)
}

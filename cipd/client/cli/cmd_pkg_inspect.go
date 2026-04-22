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
	"fmt"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// 'pkg-inspect' subcommand.

func cmdInspect() *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-inspect <package instance file>",
		ShortDesc: "inspects contents of a package instance file",
		LongDesc:  "Reads contents *.cipd file and prints information about it.",
		CommandRun: func() subcommands.CommandRun {
			c := &inspectRun{}
			c.registerBaseFlags()
			c.hashOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type inspectRun struct {
	cipdSubcommand
	hashOptions
}

func (c *inspectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(inspectInstanceFile(ctx, args[0], c.hashAlgo(), true))
}

func inspectInstanceFile(ctx context.Context, instanceFile string, hashAlgo caspb.HashAlgo, listFiles bool) (common.Pin, error) {
	inst, err := reader.OpenInstanceFile(ctx, instanceFile, reader.OpenInstanceOpts{
		VerificationMode: reader.CalculateHash,
		HashAlgo:         hashAlgo,
	})
	if err != nil {
		return common.Pin{}, err
	}
	defer inst.Close(ctx, false)
	inspectInstance(ctx, inst, listFiles)
	return inst.Pin(), nil
}

func inspectPin(ctx context.Context, pin common.Pin) {
	fmt.Printf("Instance: %s\n", pin)
}

func inspectInstance(ctx context.Context, inst pkg.Instance, listFiles bool) {
	inspectPin(ctx, inst.Pin())
	if listFiles {
		fmt.Println("Package files:")
		for _, f := range inst.Files() {
			if f.Symlink() {
				target, err := f.SymlinkTarget()
				if err != nil {
					fmt.Printf(" E %s (%s)\n", f.Name(), err)
				} else {
					fmt.Printf(" S %s -> %s\n", f.Name(), target)
				}
			} else {
				flags := make([]string, 0, 3)
				if f.Executable() {
					flags = append(flags, "+x")
				}
				if f.WinAttrs()&fs.WinAttrHidden != 0 {
					flags = append(flags, "+H")
				}
				if f.WinAttrs()&fs.WinAttrSystem != 0 {
					flags = append(flags, "+S")
				}
				flagText := ""
				if len(flags) > 0 {
					flagText = fmt.Sprintf(" (%s)", strings.Join(flags, ""))
				}
				fmt.Printf(" F %s%s\n", f.Name(), flagText)
			}
		}
	}
}

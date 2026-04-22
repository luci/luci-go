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
	"go.chromium.org/luci/common/data/stringset"
)

////////////////////////////////////////////////////////////////////////////////
// 'acl-check' subcommand.

func cmdCheckACL(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "acl-check <package subpath> [options]",
		ShortDesc: "checks whether the caller has given roles in a package",
		LongDesc:  "Checks whether the caller has given roles in a package.",
		CommandRun: func() subcommands.CommandRun {
			c := &checkACLRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.BoolVar(&c.owner, "owner", false, "Check for OWNER role.")
			c.Flags.BoolVar(&c.writer, "writer", false, "Check for WRITER role.")
			c.Flags.BoolVar(&c.reader, "reader", false, "Check for READER role.")
			return c
		},
	}
}

type checkACLRun struct {
	cipdSubcommand
	clientOptions

	owner  bool
	writer bool
	reader bool
}

func (c *checkACLRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}

	var roles []string
	if c.owner {
		roles = append(roles, "OWNER")
	}
	if c.writer {
		roles = append(roles, "WRITER")
	}
	if c.reader {
		roles = append(roles, "READER")
	}

	// By default, check for READER access.
	if len(roles) == 0 {
		roles = append(roles, "READER")
	}

	pkg, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.done(checkACL(ctx, pkg, roles, c.clientOptions))
}

func checkACL(ctx context.Context, packagePath string, roles []string, clientOpts clientOptions) (bool, error) {
	client, err := clientOpts.makeCIPDClient(ctx, "")
	if err != nil {
		return false, err
	}
	defer client.Close(ctx)

	actualRoles, err := client.FetchRoles(ctx, packagePath)
	if err != nil {
		return false, err
	}
	roleSet := stringset.NewFromSlice(actualRoles...)

	var missing []string
	for _, r := range roles {
		if !roleSet.Has(r) {
			missing = append(missing, r)
		}
	}

	if len(missing) == 0 {
		fmt.Printf("The caller has all requested role(s): %s\n", strings.Join(roles, ", "))
		return true, nil
	}

	fmt.Printf("The caller doesn't have following role(s): %s\n", strings.Join(missing, ", "))
	return false, nil
}

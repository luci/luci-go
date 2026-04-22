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

	"go.chromium.org/luci/cipd/client/cipd"
)

////////////////////////////////////////////////////////////////////////////////
// 'acl-edit' subcommand.

// principalsList is used as custom flag value. It implements flag.Value.
type principalsList []string

func (l *principalsList) String() string {
	return fmt.Sprintf("%v", *l)
}

func (l *principalsList) Set(value string) error {
	// Ensure <type>:<id> syntax is used. Let the backend to validate the rest.
	chunks := strings.Split(value, ":")
	if len(chunks) != 2 {
		return makeCLIError("%q doesn't look like principal id (<type>:<id>)", value)
	}
	*l = append(*l, value)
	return nil
}

func cmdEditACL(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "acl-edit <package subpath> [options]",
		ShortDesc: "modifies package path Access Control List",
		LongDesc:  "Modifies package path Access Control List.",
		CommandRun: func() subcommands.CommandRun {
			c := &editACLRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.Var(&c.owner, "owner", "Users (user:email) or groups (`group:name`) to grant OWNER role.")
			c.Flags.Var(&c.writer, "writer", "Users (user:email) or groups (`group:name`) to grant WRITER role.")
			c.Flags.Var(&c.reader, "reader", "Users (user:email) or groups (`group:name`) to grant READER role.")
			c.Flags.Var(&c.revoke, "revoke", "Users (user:email) or groups (`group:name`) to remove from all roles.")
			return c
		},
	}
}

type editACLRun struct {
	cipdSubcommand
	clientOptions

	owner  principalsList
	writer principalsList
	reader principalsList
	revoke principalsList
}

func (c *editACLRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	pkg, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.done(nil, editACL(ctx, pkg, c.owner, c.writer, c.reader, c.revoke, c.clientOptions))
}

func editACL(ctx context.Context, packagePath string, owners, writers, readers, revoke principalsList, clientOpts clientOptions) error {
	changes := []cipd.PackageACLChange{}

	makeChanges := func(action cipd.PackageACLChangeAction, role string, list principalsList) {
		for _, p := range list {
			changes = append(changes, cipd.PackageACLChange{
				Action:    action,
				Role:      role,
				Principal: p,
			})
		}
	}

	makeChanges(cipd.GrantRole, "OWNER", owners)
	makeChanges(cipd.GrantRole, "WRITER", writers)
	makeChanges(cipd.GrantRole, "READER", readers)

	makeChanges(cipd.RevokeRole, "OWNER", revoke)
	makeChanges(cipd.RevokeRole, "WRITER", revoke)
	makeChanges(cipd.RevokeRole, "READER", revoke)

	if len(changes) == 0 {
		return nil
	}

	client, err := clientOpts.makeCIPDClient(ctx, "")
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	err = client.ModifyACL(ctx, packagePath, changes)
	if err != nil {
		return err
	}
	fmt.Println("ACL changes applied.")
	return nil
}

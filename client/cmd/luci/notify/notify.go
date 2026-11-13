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

// Package notify implements the `luci notify` CLI subcommands.
package notify

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	lucinotifypb "go.chromium.org/luci/luci_notify/api/service/v1"
)

// Cmd returns the top-level subcommand for `luci notify`.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "notify <subcommand>",
		ShortDesc: "Query LUCI Notify builder statuses",
		LongDesc: "Query LUCI Notify (notify.api.luci.app) builder statuses.\n\n" +
			"Available subcommands:\n" +
			"  builder   List builders and their latest status (e.g. 'luci notify builder list -rotation chrome_browser_release -status FAILURE')",
		CommandRun: func() subcommands.CommandRun {
			return &notifyRun{af: af}
		},
	}
}

type notifyRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *notifyRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci notify", "LUCI Notify inspection", []*subcommands.Command{
		BuilderCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

func ensureNotifyClients(ctx context.Context, af *base.AuthFlags, host string, bc lucinotifypb.BuildersClient, agc lucinotifypb.AlertGroupsClient) (lucinotifypb.BuildersClient, lucinotifypb.AlertGroupsClient, bool) {
	if bc != nil || agc != nil {
		return bc, agc, true
	}
	if af == nil {
		fmt.Fprintf(os.Stderr, "auth flags not configured\n")
		return nil, nil, false
	}
	if err := af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return nil, nil, false
	}
	var err error
	bc, agc, err = af.NewNotifyClients(ctx, host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return nil, nil, false
	}
	return bc, agc, true
}

func trimRotationPrefix(raw string) string {
	s := strings.ToLower(strings.Trim(strings.TrimSpace(raw), "/"))
	if s == "rotations" {
		return ""
	}
	return strings.TrimPrefix(s, "rotations/")
}

// Copyright 2020 The LUCI Authors.
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
	"os"
	"os/exec"
	"path/filepath"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/resultdb/pbutil"
)

const (
	ExitCodeSuccess = iota
	ExitCodeInvalidInput
	ExitCodeInternalError
)

type contextRun struct {
	subcommands.CommandRunBase

	host         string
	invocationID string
	updateToken  string
}

func cmdContext() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `context [flags] <other command to run in the rdb context>`,
		ShortDesc: "run another command inside a specified rdb context.",
		Advanced:  true,
		LongDesc: text.Doc(`
			Ensure a luci context exists and set the values of its resultdb
			section to those indicated by the flags given. Then export this
			into the environment that the inner command will be run in.

			e.g. "rdb context -host rdb.dev -inv build:123 -token abc rdb update-inclusions -add build:345"
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &contextRun{}
			r.RegisterFlags()
			return r
		},
	}
}

func (r *contextRun) RegisterFlags() {
	r.Flags.StringVar(&r.host, "host", "", text.Doc(`
		Host of the resultdb instance.
	`))
	r.Flags.StringVar(&r.invocationID, "inv", "", text.Doc(`
		ID for resultdb.current_invocation.name in luci context.
		e.g. build:123456
	`))
	r.Flags.StringVar(&r.updateToken, "token", "", text.Doc(`
		Value for resultdb.current_invocation.update_token in luci context.
	`))
}

func (r *contextRun) populateResultDBContext(ctx context.Context) context.Context {
	rdbCtx := lucictx.GetResultDB(ctx)
	if rdbCtx == nil {
		rdbCtx = &lucictx.ResultDB{}
	}
	if r.host != "" {
		rdbCtx.Hostname = r.host
	}
	if r.invocationID != "" {
		rdbCtx.CurrentInvocation.Name = pbutil.InvocationName(r.invocationID)
	}
	if r.updateToken != "" {
		rdbCtx.CurrentInvocation.UpdateToken = r.updateToken
	}
	return lucictx.SetResultDB(ctx, rdbCtx)
}

func (r *contextRun) makeCmd(args []string) (*exec.Cmd, error) {
	if len(args) == 0 {
		return nil, errors.Reason("Specify a command to run:\n rdb context [flags] [--] <bin> [args]").Err()
	}
	bin := args[0]
	if filepath.Base(bin) == bin {
		resolved, err := exec.LookPath(bin)
		if err != nil {
			return nil, errors.Reason("Can't find %q in PATH", bin).Err()
		}
		bin = resolved
	}
	return &exec.Cmd{
		Path:   bin,
		Args:   args,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, nil

}

func (r *contextRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := r.populateResultDBContext(cli.GetContext(a, r, env))

	cmd, err := r.makeCmd(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeInvalidInput
	}

	exported, err := lucictx.Export(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting LUCI_CONTEXT %s", err)
		return ExitCodeInternalError
	}
	defer exported.Close()
	exported.SetInCmd(cmd)

	logging.Debugf(ctx, "Running %q", cmd.Args)
	if err := cmd.Run(); err != nil {
		if code, hasCode := exitcode.Get(err); hasCode {
			return code
		}
		return ExitCodeInternalError
	}
	return ExitCodeSuccess
}

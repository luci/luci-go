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
	"io/ioutil"
	"os"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
)

const callUsage = `call [flags] SERVICE METHOD`

func cmdCall(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: callUsage,
		ShortDesc: "call a ResultDB RPC",
		LongDesc: text.Doc(`
			Call a ResultDB RPC.

			SERVICE must be the full name of a service, e.g. "luci.resultdb.rpc.v1.ResultDB"".
			METHOD is the name of the method, e.g. "GetInvocation"

			The request message is read from stdin, in JSON format.
			The response is printed to stdout, also in JSON format.
		`),
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			r := &callRun{}
			r.RegisterGlobalFlags(p)
			return r
		},
	}
}

type callRun struct {
	baseCommandRun
	service string
	method  string
}

func (r *callRun) parseArgs(args []string) error {
	if len(args) != 2 {
		return errors.Reason("usage: %s", callUsage).Err()
	}

	r.service = args[0]
	r.method = args[1]

	return nil
}

func (r *callRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(err)
	}

	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	if err := r.call(ctx); err != nil {
		return r.done(err)
	}

	return 0
}

func (r *callRun) call(ctx context.Context) error {
	// Prepare arguments.
	in, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	if r.resultdbCtx.CurrentInvocation.UpdateToken != "" {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
			"update-token", r.resultdbCtx.CurrentInvocation.UpdateToken))
	}

	// Send the request.
	res, err := r.prpcClient.CallWithFormats(ctx, r.service, r.method, in, prpc.FormatJSONPB, prpc.FormatJSONPB)
	if err != nil {
		return err
	}

	// Read response.
	if _, err := os.Stdout.Write(res); err != nil {
		return fmt.Errorf("failed to write response: %s", err)
	}

	return nil
}

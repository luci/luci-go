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
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/exec2"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/resultdb/sink"
)

func cmdStream(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `stream [flags] TEST_CMD [TEST_ARG]...`,
		ShortDesc: "Run a given test command and upload the results to ResultDB",
		// TODO(crbug.com/1017288): add a link to ResultSink protocol doc
		LongDesc: text.Doc(`
			Run a given test command, continuously collect the results over IPC, and
			upload them to ResultDB. Either use the current invocation from
			LUCI_CONTEXT or create/finalize a new one. Example:
				rdb stream ./out/chrome/test/browser_tests
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &streamRun{}
			r.baseCommandRun.RegisterGlobalFlags(p)
			r.Flags.BoolVar(&r.isNew, "new", false, text.Doc(`
				If true, create and use a new invocation for the test command.
				If false, use the current invocation, set in LUCI_CONTEXT.
			`))
			return r
		},
	}
}

type streamRun struct {
	baseCommandRun

	// flags
	isNew bool
	// TODO(ddoman): add flags
	// - testPathPrefix
	// - tag (invocation-tag)
	// - var (base-test-variant)
	// - complete-invocation-exit-codes
	// - log-file

	invocation lucictx.Invocation
}

func (r *streamRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	// init client
	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	// if -new is passed, create a new invocation. If not, use the existing one set in
	// lucictx.
	if r.isNew {
		// TODO(ddoman): create an invocation
		return r.done(errors.Reason("Not implemented").Err())
	} else {
		if err := r.validateCurrentInvocation(); err != nil {
			return r.done(err)
		}
		r.invocation = r.resultdbCtx.CurrentInvocation
	}

	// run the command
	cmd, err := r.makeTestCmd(ctx, args)
	if err != nil {
		return r.done(err)
	}
	testExitCode, err := r.runTestCmd(ctx, cmd)

	// TODO(ddoman): finalize the invocation
	if err != nil {
		return r.done(err)
	}
	return testExitCode
}

func (r *streamRun) makeTestCmd(ctx context.Context, args []string) (*exec2.Cmd, error) {
	if len(args) == 0 {
		return nil, errors.Reason("missing a test command to run").Err()
	}
	cmd := exec2.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd, nil
}

func (r *streamRun) runTestCmd(ctx context.Context, cmd *exec2.Cmd) (int, error) {
	// Set the server configs based on the flags and lucictx
	server := sink.NewServer(sink.ServerConfig{
		Recorder:    r.recorder,
		Invocation:  r.invocation.Name,
		UpdateToken: r.invocation.UpdateToken,
	})

	// reset and install a lucictx with r.host and r.invocation just in case they were not
	// derived from the current lucictx.
	ctx = lucictx.SetResultDB(ctx, &lucictx.ResultDB{
		Hostname:          r.host,
		CurrentInvocation: r.invocation,
	})
	exported, err := lucictx.Export(ctx)
	if err != nil {
		return -1, errors.Annotate(err, "exporting LUCI_CONTEXT").Err()
	}
	defer exported.Close()
	exported.SetInCmd(cmd.Cmd)

	// TODO(ddoman): send the logs of SinkServer to --log-file
	err = server.Run(ctx, func(ctx context.Context) error {
		logging.Debugf(ctx, "Starting: %q", cmd.Args)
		if err := cmd.Start(); err != nil {
			return errors.Annotate(err, "cmd.start").Err()
		}
		return cmd.Wait()
	})
	ec, ok := exitcode.Get(err)
	if !ok {
		return -1, err
	}
	logging.Infof(ctx, "Child process terminated with %d", ec)
	return ec, nil
}

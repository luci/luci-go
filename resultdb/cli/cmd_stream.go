// Copyright 2019 The LUCI Authors.
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

const (
	// StreamExitCodeInvalidInput is the code `stream` exits with, when it fails to run
	// due to invalid command-line inputs.
	StreamExitCodeInvalidInput = 1001
	// StreamExitCodeInternalError is the code `stream` exits with, when it fails to run
	// due to internal errors.
	StreamExitCodeInternalError = 1002
)

// streamCommandError is a builder for associating errors from subcommand `stream` with
// exit codes.
type streamCommandError struct {
	inner    error
	ExitCode int
}

func (se *streamCommandError) Error() string {
	return se.inner.Error()
}

func invalidInput(err error) error {
	return &streamCommandError{err, StreamExitCodeInvalidInput}
}

func internalError(err error) error {
	return &streamCommandError{err, StreamExitCodeInternalError}
}

func cmdStream(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `stream [flags] TEST_CMD [TEST_ARG]...`,
		ShortDesc: "Run a given test command and upload the results to ResultDB",
		LongDesc: text.Doc(`
			Run a given test command, continuously collect the results over IPC and upload
			them to ResultDB. Either use the current invocation from LUCI_CONTEXT or
			create/finalize a new one. Example:
				rdb stream ./out/chrome/test/browser_tests
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &streamRun{}
			r.baseCommandRun.RegisterGlobalFlags(p)
			r.Flags.BoolVar(&r.isNew, "new", false, text.Doc(`
				If true, create and use a new invocation for the test run.
				If false, the current invocation must be set in LUCI_CONTEXT
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

	invocation lucictx.Invocation
}

func (r *streamRun) makeTestCmd(ctx context.Context, args []string) (*exec2.Cmd, error) {
	if len(args) == 0 {
		return nil, invalidInput(errors.Reason("missing a test command to run").Err())
	}
	cmd := exec2.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// install a lucictx into the test command
	exported, err := lucictx.Export(ctx)
	if err != nil {
		return nil, internalError(errors.Annotate(err, "exporting LUCI_CONTEXT").Err())
	}
	defer exported.Close()
	exported.SetInCmd(cmd.Cmd)

	return cmd, nil
}

func failed(ctx context.Context, err error) int {
	code := 1
	if se, ok := err.(*streamCommandError); ok {
		code = se.ExitCode
	}
	logging.Errorf(ctx, "%s", err)
	return code
}

func (r *streamRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	// init client
	if err := r.initClients(ctx); err != nil {
		return failed(ctx, invalidInput(err))
	}

	// if the invocation is missing in lucictx, create a new invocation for the test run.
	if err := r.validateCurrentInvocation(); err != nil {
		if !r.isNew {
			return failed(ctx, invalidInput(err))
		}
		// TODO(ddoman): create an invocation
		return failed(ctx, internalError(errors.Reason("Not implemented").Err()))
	} else {
		r.invocation = r.resultdbCtx.CurrentInvocation
	}

	// reset the lucictx just in case if r.host and r.invocation were not derived from
	// lucictx.
	ctx = lucictx.SetResultDB(ctx, &lucictx.ResultDB{
		Hostname:          r.host,
		CurrentInvocation: r.invocation,
	})
	cmd, err := r.makeTestCmd(ctx, args)
	if err != nil {
		return failed(ctx, internalError(err))
	}
	ec, err := r.runTestCmd(ctx, cmd)
	if err != nil {
		return failed(ctx, err)
	}
	return ec
}

func (r *streamRun) runTestCmd(ctx context.Context, cmd *exec2.Cmd) (int, error) {
	// Set the server configs based on the flags and lucictx
	server := sink.NewServer(sink.ServerConfig{
		Recorder:    r.recorder,
		Invocation:  r.invocation.Name,
		UpdateToken: r.invocation.UpdateToken,
	})

	var ec int
	var ok bool
	err := server.Run(ctx, func(ctx context.Context) error {
		logging.Debugf(ctx, "Running %q", cmd.Args)
		if err := cmd.Start(); err != nil {
			return internalError(errors.Annotate(err, "cmd.start").Err())
		}
		err := cmd.Wait()
		if ec, ok = exitcode.Get(err); !ok {
			return internalError(errors.Annotate(err, "cmd.wait").Err())
		}
		return nil
	})

	logging.Infof(ctx, "Child process terminated with %d", ec)
	return ec, err
}

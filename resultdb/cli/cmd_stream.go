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
	"encoding/base64"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/exec2"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/resultdb/internal/services/recorder"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	"go.chromium.org/luci/resultdb/sink"
)

var matchInvalidInvocationIDChars = regexp.MustCompile(`[^a-z0-9_\-:.]`)

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

func (r *streamRun) Run(a subcommands.Application, args []string, env subcommands.Env) (ret int) {
	ctx := cli.GetContext(a, r, env)
	if len(args) == 0 {
		return r.done(errors.Reason("missing a test command to run").Err())
	}
	cmd := exec2.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// init client
	if err := r.initClients(ctx); err != nil {
		return r.done(err)
	}

	// if -new is passed, create a new invocation. If not, use the existing one set in
	// lucictx.
	if r.isNew {
		ninv, err := r.createInvocation(ctx)
		if err != nil {
			return r.done(err)
		}
		r.invocation = ninv
	} else {
		if err := r.validateCurrentInvocation(); err != nil {
			return r.done(err)
		}
		r.invocation = r.resultdbCtx.CurrentInvocation
	}

	interrupted := true
	defer func() {
		// finalize the invocation if it was created by -new or the test command execution
		// was interrupted.
		if r.isNew || interrupted {
			if err := r.finalizeInvocation(ctx, interrupted); err != nil {
				logging.Errorf(ctx, "failed to finalize the invocation: %s", err)
				ret = r.done(err)
			}
		}
	}()

	err := r.runTestCmd(ctx, cmd)
	ec, ok := exitcode.Get(err)
	if !ok {
		return r.done(err)
	}
	// TODO(ddoman): handle complete-invocation-exit-codes
	interrupted = ec != 0
	logging.Infof(ctx, "Child process terminated with %d", ec)
	return ec
}

func (r *streamRun) runTestCmd(ctx context.Context, cmd *exec2.Cmd) error {
	// reset and install a lucictx with r.host and r.invocation just in case they were not
	// derived from the current lucictx.
	ctx = lucictx.SetResultDB(ctx, &lucictx.ResultDB{
		Hostname:          r.host,
		CurrentInvocation: r.invocation,
	})
	exported, err := lucictx.Export(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to export LUCI_CONTEXT").Err()
	}
	defer exported.Close()
	exported.SetInCmd(cmd.Cmd)

	// TODO(ddoman): send the logs of SinkServer to --log-file
	// TODO(ddoman): handle interrupts with luci/common/system/signals.
	cfg := sink.ServerConfig{
		Recorder:    r.recorder,
		Invocation:  r.invocation.Name,
		UpdateToken: r.invocation.UpdateToken,
	}
	return sink.Run(ctx, cfg, func(ctx context.Context, cfg sink.ServerConfig) error {
		// install the lucicontext into cmd again so that all the lucictx added inside
		// the server becomes visible to the test process.
		exported, err := lucictx.Export(ctx)
		if err != nil {
			return err
		}
		defer exported.Close()
		exported.SetInCmd(cmd.Cmd)

		logging.Debugf(ctx, "Starting: %q", cmd.Args)
		if err := cmd.Start(); err != nil {
			return errors.Annotate(err, "cmd.start").Err()
		}
		return cmd.Wait()
	})
}

func (r *streamRun) createInvocation(ctx context.Context) (ret lucictx.Invocation, err error) {
	invID, err := genInvID(ctx)
	if err != nil {
		return
	}

	md := metadata.MD{}
	resp, err := r.recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{
		InvocationId: invID,
	}, prpc.Header(&md))
	if err != nil {
		err = errors.Annotate(err, "failed to create an invocation").Err()
		return
	}
	tks := md.Get(recorder.UpdateTokenMetadataKey)
	if len(tks) == 0 {
		err = errors.Reason("Missing header: update-token").Err()
		return
	}

	ret = lucictx.Invocation{resp.Name, tks[0]}
	fmt.Fprintf(os.Stderr, "created invocation: %s\n", invID)
	return
}

// finalizeInvocation finalizes the invocation.
func (r *streamRun) finalizeInvocation(ctx context.Context, interrupted bool) error {
	ctx = metadata.AppendToOutgoingContext(
		ctx, recorder.UpdateTokenMetadataKey, r.invocation.UpdateToken)
	_, err := r.recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{
		Name:        r.invocation.Name,
		Interrupted: interrupted,
	})
	return err
}

// genInvID generates an invocation ID, made of the username, the current timestamp
// in a human-friendly format, and a random suffix.
//
// This can be used to generate a random invocation ID, but the creator and creation time
// can be easily found.
func genInvID(ctx context.Context) (string, error) {
	whoami, err := user.Current()
	if err != nil {
		return "", err
	}
	bytes := make([]byte, 8)
	if _, err := mathrand.Read(ctx, bytes); err != nil {
		return "", err
	}

	username := strings.ToLower(whoami.Username)
	username = matchInvalidInvocationIDChars.ReplaceAllString(username, "")

	suffix := strings.ToLower(fmt.Sprintf(
		"%s:%s", time.Now().UTC().Format(time.RFC3339),
		// Use unpadded encoding because the padding character,'=', is not a valid character
		// for invocation IDs and also shorter.
		base64.RawURLEncoding.EncodeToString(bytes)))

	// An invocation ID can contain up to 100 ascii characters that conform to the regex,
	return fmt.Sprintf("u:%.*s:%s", 100-len(suffix), username, suffix), nil
}

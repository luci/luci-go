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

package wrapper

import (
	"context"
	"flag"
	"fmt"
	"os"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/exec2"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	"go.chromium.org/luci/resultdb/sink"
)

// Wrapper launches a sink server and runs the test command under the sink server.
//
// If the test command run finishes, the sink server terminates and Wrapper returns
// the exit code of the child process for the test command.
type Wrapper struct {
	serverCfg     sink.ServerConfig
	logFile       *os.File
	logCfg        gologger.LoggerConfig
	cmdName       string
	cmdArgs       []string
	childExitCode int
}

func NewWrapper(ctx context.Context) (*Wrapper, error) {
	w := &Wrapper{}
	flgs, err := parseFlags()
	if err != nil {
		return nil, err
	}
	if err := w.initServerConfigs(ctx, flgs); err != nil {
		return nil, err
	}

	// parse cmd arguments for the test
	if flag.NArg() < 1 {
		return nil, errors.Reason("must pass command to run in wrapper").Err()
	}
	w.cmdName = flag.Arg(0)
	w.cmdArgs = flag.Args()[1:]

	if flgs.logFile == "" {
		w.logCfg.Out = os.Stderr
	} else {
		f, err := os.OpenFile(flgs.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		w.logFile = f
		w.logCfg.Out = w.logFile
	}
	// TODO(sajjadm): Parse remaining flags into meaningful structures
	return w, nil
}

func (w *Wrapper) initServerConfigs(ctx context.Context, flgs *flags) error {
	inv := lucictx.GetResultDB(ctx).CurrentInvocation
	if err := pbutil.ValidateInvocationName(inv.Name); err != nil {
		return errors.Annotate(err, "lucictx.ResultDB.CurrentInvocation:Name").Err()
	}

	// Create pRPC over HTTP clients.
	hc, err := auth.NewAuthenticator(
		ctx, auth.SilentLogin, chromeinfra.DefaultAuthOptions()).Client()
	if err != nil {
		return err
	}
	info, err := version.GetCurrentVersion()
	if err != nil {
		return err
	}
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.UserAgent = fmt.Sprintf("result_sink_wrapper, instanceID=%q", info.InstanceID)
	pc := &prpc.Client{
		C:       hc,
		Host:    flgs.recorder,
		Options: rpcOpts,
	}

	// Set the server configs based on the flags and lucictx
	w.serverCfg = sink.ServerConfig{
		Address:      flgs.sink,
		Recorder:     pb.NewRecorderPRPCClient(pc),
		TestIDPrefix: flgs.testIDPrefix,
		Invocation:   inv.Name,
		UpdateToken:  inv.UpdateToken,
	}
	return nil
}

func (w *Wrapper) Close() {
	if w.logFile != nil {
		w.logFile.Close()
	}
}

func (w *Wrapper) run(ctx context.Context) (int, error) {
	// TODO(crbug.com/1017288) Export server information into child's context.
	cmd := exec2.CommandContext(ctx, w.cmdName, w.cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return -1, errors.Annotate(err, "could not start subprocess").Err()
	}
	if err := cmd.Wait(); err != nil {
		if code, ok := exitcode.Get(err); ok {
			return code, nil
		}
		return -1, errors.Annotate(err, "failed to wait for subprocess").Err()
	}

	return 0, nil
}

func (w *Wrapper) Main(ctx context.Context) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = w.logCfg.Use(ctx)

	// TODO(sajjadm): Use https://godoc.org/go.chromium.org/luci/common/system/signals
	// to handle interrupts

	server := sink.NewServer(w.serverCfg)
	childExitCode := -1
	err := server.Run(ctx, func(ctx context.Context) (err error) {
		childExitCode, err = w.run(ctx)
		return err
	})
	logging.Infof(ctx, "Child process terminated with %d", childExitCode)
	return childExitCode, err
}

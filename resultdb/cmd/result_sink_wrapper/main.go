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

package main

import (
	"context"
	"fmt"
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/exec2"
	"go.chromium.org/luci/common/system/exitcode"

	"go.chromium.org/luci/resultdb/sink"
)

const (
	wrapperErrorCode = 1001
)

type wrapper struct {
	serverCfg     sink.ServerConfig
	cmdName       string
	cmdArgs       []string
	childExitCode int
}

func (w *wrapper) run(ctx context.Context) (int, error) {
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

func (w *wrapper) main(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// TODO(sajjadm): Use https://godoc.org/go.chromium.org/luci/common/system/signals
	// to handle interrupts

	server, err := sink.NewServer(ctx, w.serverCfg)
	if err != nil {
		return err
	}

	err = server.Run(ctx, func(ctx context.Context) error {
		code, err := w.run(ctx)
		if err != nil {
			return err
		}
		w.childExitCode = code
		return nil
	})
	if err != nil {
		return err
	}

	// TODO(sajjadm): Add post-command work such as uploading from results file

	return nil
}

func initLogs(flags *sinkWrapperFlags) (context.Context, func(), error) {
	lcfg := gologger.LoggerConfig{Out: os.Stderr}
	cleanup := func() {}
	if flags.LogFile != "" {
		o, err := os.OpenFile(flags.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, cleanup, err
		}
		lcfg.Out = o
		cleanup = func() {
			o.Sync()
			o.Close()
		}
	}
	return flags.LoggingConfig.Set(lcfg.Use(context.Background())), cleanup, nil
}

func fatalf(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(wrapperErrorCode)
}

func main() {
	flags, args, err := parseFlags()
	if err != nil {
		fatalf(err)
	}
	ctx, cleanup, err := initLogs(flags)
	if err != nil {
		fatalf(err)
	}
	defer cleanup()

	var exitCode int
	w := wrapper{cmdName: args[0], cmdArgs: args[1:]}
	if err := w.main(ctx); err != nil {
		logging.Errorf(ctx, "FATAL: %s", err)
		exitCode = wrapperErrorCode
	} else {
		exitCode = w.childExitCode
	}
	os.Exit(exitCode)
}

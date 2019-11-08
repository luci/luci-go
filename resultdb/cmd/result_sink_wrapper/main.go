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
	"flag"
	"fmt"
	"os"
	"strconv"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/exec2"
	"go.chromium.org/luci/common/system/exitcode"

	"go.chromium.org/luci/resultdb/sink"
)

const (
	wrapperErrorCode = 1001
)

type wrapperArgs struct {
	port                        int
	recorder                    string
	invocationIDFile            string
	logFile                     string
	resultFile                  []string
	testPathPrefix              string
	invocationTags              strpair.Map
	testBaseVariant             strpair.Map
	completeInvocationExitCodes []int
}

func parseArgs() (wrapperArgs, error) {
	var args wrapperArgs

	flag.IntVar(&args.port, "port", 0, "TCP port to listen on")
	// TODO(sajjadm): Set the default recorder once it exists.
	flag.StringVar(&args.recorder, "recorder", "",
		"Address, hostname, or other identifier off the Recorder service that the server should upload results to")

	flag.StringVar(&args.invocationIDFile, "invocation-id-file", "",
		"Path to write the generated invocation ID")

	flag.StringVar(&args.logFile, "log-file", "", "File to log to")

	flag.Var(luciflag.StringSlice(&args.resultFile), "result-file",
		"Files to read and upload after running the subprocess, may be set more than once")

	flag.StringVar(&args.testPathPrefix, "test-path-prefix", "",
		"Prefix to prepepend before the test path of every test result")

	args.invocationTags = make(strpair.Map)
	flag.Var(luciflag.StringPairs(args.invocationTags), "invocation-tag",
		"Tag to add to the Invocation, of form key:value, may be set more than once")

	args.testBaseVariant = make(strpair.Map)
	flag.Var(luciflag.StringPairs(args.testBaseVariant), "test-variant",
		"Variant to add to each test, of form key:value, may be set more than once")

	var rawExitCodes []string
	flag.Var(luciflag.CommaList(&rawExitCodes),
		"complete-invocation-exit-codes",
		"Comma-separated list of exit codes from the subprocess that mean the Invocation should be marked completed, otherwise it will be marked interrupted")

	flag.Parse()

	args.completeInvocationExitCodes = make([]int, 0, len(rawExitCodes))
	for _, rawCode := range rawExitCodes {
		if code, err := strconv.Atoi(rawCode); err != nil {
			return args, errors.Annotate(err, "must pass integers to -complete-invocation-exit-codes").Err()
		} else {
			args.completeInvocationExitCodes = append(args.completeInvocationExitCodes, code)
		}
	}

	return args, nil
}

type wrapper struct {
	serverCfg     sink.ServerConfig
	logFile       *os.File
	logCfg        gologger.LoggerConfig
	cmdName       string
	cmdArgs       []string
	childExitCode int
}

func (w *wrapper) init() error {
	args, err := parseArgs()
	if err != nil {
		return err
	}

	if flag.NArg() < 1 {
		return errors.Reason("must pass command to run in wrapper").Err()
	}
	w.cmdName = flag.Arg(0)
	w.cmdArgs = flag.Args()[1:]

	w.serverCfg.Port = args.port

	if args.logFile != "" {
		f, err := os.OpenFile(args.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		w.logFile = f
		w.logCfg.Out = w.logFile
	} else {
		w.logCfg.Out = os.Stderr
	}

	// TODO(sajjadm): Parse remaining args into meaningful structures

	return nil
}

func (w *wrapper) close() {
	if w.logFile != nil {
		w.logFile.Close()
	}
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
	server, err := sink.NewServer(ctx, w.serverCfg)
	if err != nil {
		return err
	}

	server.Run(ctx, func(ctx context.Context) error {
		if code, err := w.run(ctx); err != nil {
			return err
		} else {
			w.childExitCode = code
			return nil
		}
	})

	// TODO(sajjadm) Add post-command work such as uploading from results file

	return nil
}

func main() {
	var w wrapper
	if err := w.init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(wrapperErrorCode)
	}

	ctx := w.logCfg.Use(context.Background())
	ctx = logging.SetLevel(ctx, logging.Debug)

	var exitCode int
	if err := w.main(ctx); err != nil {
		logging.Errorf(ctx, "FATAL: %s", err)
		exitCode = wrapperErrorCode
	} else {
		exitCode = w.childExitCode
	}

	w.close()
	os.Exit(exitCode)
}

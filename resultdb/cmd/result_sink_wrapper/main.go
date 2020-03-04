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
	resultFiles                 strpair.Map
	testIDPrefix                string
	invocationTags              strpair.Map
	baseTestVariant             map[string]string
	completeInvocationExitCodes []int
}

func parseArgs() (wrapperArgs, error) {
	args := wrapperArgs{
		invocationTags: make(strpair.Map),
		resultFiles:    make(strpair.Map),
	}

	flag.IntVar(&args.port, "port", 0, "TCP port to listen on, will be arbitrarily selected if unset")
	// TODO(sajjadm): Set the default recorder once it exists.
	flag.StringVar(&args.recorder, "recorder", "",
		"Hostname of the Recorder service that the server should upload results to")

	flag.StringVar(&args.invocationIDFile, "invocation-id-file", "",
		"Path to write the generated invocation ID")

	flag.StringVar(&args.logFile, "log-file", "", "File to log to")

	flag.Var(luciflag.StringPairs(args.resultFiles), "result-file",
		"Files to read and upload after running the subprocess, of form format:path, may be set more than once. Valid formats are luci, chromium_jtr, and chromium_gtest")

	flag.StringVar(&args.testIDPrefix, "test-id-prefix", "",
		"Prefix to prepepend before the test id of every test result")

	flag.Var(luciflag.StringPairs(args.invocationTags), "invocation-tag",
		"Tag to add to the Invocation, of form key:value, may be set more than once")

	// TODO(sajjadm): Add new function to flag package that decodes to a map[string]string and
	// enforces unique keys, then use that function to implement a -base-test-variant flag.
	// The description for the flag will be:
	// "Variant definition pairs common for all test results in this Invocation, of form key:value"
	// It will decode into wrapperArgs.baseTestVariant

	var rawExitCodes []string
	flag.Var(luciflag.CommaList(&rawExitCodes),
		"complete-invocation-exit-codes",
		"Comma-separated list of exit codes from the subprocess that mean the Invocation should be marked non-interrupted")

	flag.Parse()

	args.completeInvocationExitCodes = make([]int, len(rawExitCodes))
	for i, rawCode := range rawExitCodes {
		code, err := strconv.Atoi(rawCode)
		if err != nil {
			return args, errors.Annotate(err, "must pass integers to -complete-invocation-exit-codes").Err()
		}
		args.completeInvocationExitCodes[i] = code
	}

	if len(args.completeInvocationExitCodes) == 0 {
		args.completeInvocationExitCodes = []int{0}
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

	w.serverCfg.Address = fmt.Sprint(":", args.port)

	if args.logFile == "" {
		w.logCfg.Out = os.Stderr
	} else {
		f, err := os.OpenFile(args.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		w.logFile = f
		w.logCfg.Out = w.logFile
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// TODO(sajjadm): Use https://godoc.org/go.chromium.org/luci/common/system/signals
	// to handle interrupts

	server := sink.NewServer(w.serverCfg)
	err := server.Run(ctx, func(ctx context.Context) error {
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

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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/exec2"
	"go.chromium.org/luci/resultdb/sink"
)

type wrapperArgs struct {
	port                        int
	recorder                    string
	invocationIDFile            string
	logFile                     string
	resultFile                  string
	testPathPrefix              string
	invocationTags              string
	testBaseVariant             string
	completeInvocationExitCodes string
}

func parseArgs() wrapperArgs {
	var args wrapperArgs
	flag.IntVar(&args.port, "port", 0, "TCP port to listen on.")
	flag.StringVar(&args.recorder, "recorder", "TODO",
		"Address, hostname, or other identifier off the Recorder service that the server should upload results to.")
	flag.StringVar(&args.invocationIDFile, "invocation-id-file", "",
		"Path to write the generated invocation ID.")
	flag.StringVar(&args.logFile, "log-file", "", "File to log to.")
	flag.StringVar(&args.resultFile, "result-file", "",
		"File to read and upload after the subprocess exits.")
	flag.StringVar(&args.testPathPrefix, "test-path-prefix", "",
		"Prefix to prepepend before the test path of every test result.")
	flag.StringVar(&args.invocationTags, "invocation-tags", "",
		"Tags to add to the created invocation.")
	flag.StringVar(&args.testBaseVariant, "test-base-variant", "",
		"Variant to use for the test results.")
	flag.StringVar(&args.completeInvocationExitCodes,
		"complete-invocation-exit-codes", "",
		"Exit codes from the subprocess that indicate the invocation is complete.")

	flag.Parse()
	return args
}

type wrapper struct {
	port    int
	logFile *os.File
	logCfg  gologger.LoggerConfig
	cmdName string
	cmdArgs []string
}

func (w *wrapper) init() error {
	args := parseArgs()

	if flag.NArg() < 1 {
		return errors.Reason("must pass command to run in wrapper").Err()
	}
	w.cmdName = flag.Arg(0)
	if flag.NArg() > 1 {
		w.cmdArgs = flag.Args()[1:]
	}

	w.port = args.port

	if args.logFile != "" {
		f, err := os.OpenFile(args.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		w.logFile = f
		w.logCfg.Out = w.logFile
	}

	return nil
}

func (w *wrapper) close() {
	if w.logFile != nil {
		w.logFile.Close()
	}
}

func main() {
	baseCtx := context.Background()
	var w wrapper
	defer w.close()
	if err := w.init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	ourCtx := w.logCfg.Use(baseCtx)

	server, err := sink.NewServer(ourCtx, sink.ServerConfig{Port: w.port})
	if err != nil {
		logging.Errorf(ourCtx, "Could not create sink.Server: %s", err)
		return
	}
	server.Serve(ourCtx)
	defer server.Close(ourCtx)

	// TODO(crbug.com/1017288) Export server information into child's context.
	childCtx := baseCtx
	cmd := exec2.CommandContext(childCtx, w.cmdName, w.cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		logging.Errorf(ourCtx, "Could not start subprocess: %s", err)
		return
	}
	if err := cmd.Wait(); err != nil {
		logging.Errorf(ourCtx, "Failed to wait for subprocess: %s", err)
		return
	}
}

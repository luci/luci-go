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

package main

import (
	"flag"
	"os"

	"go.chromium.org/luci/common/data/strpair"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"
)

// sinkWrapperFlags defines command line flags related to result_sink_wrapper.
// Use NewWrapperFlags() to get a sinkWrapperFlags struct with sensible default values.
type sinkWrapperFlags struct {
	// flags for the test results
	InvocationIDFile string
	TestIDPrefix     string
	TestBaseVariants strpair.Map
	TestResultTags   strpair.Map

	CompleteInvocationExitCodes []int32

	// flags for the SinkServer
	SinkAddress string
	// TODO(ddoman): move this to sink/flags.go so that the flags become available
	// to whichever binaries importing SinkServer.
	Recorder    string

	// flags for logging
	LogFile       string
	LoggingConfig logging.Config
}

// NewFlags returns a Flags struct with sensible default values.
func newFlags() *sinkWrapperFlags {
	return &sinkWrapperFlags{
		CompleteInvocationExitCodes: []int32{0},

		// TODO(ddoman): Set the default value with sink.DefaultAddress.
		SinkAddress: "localhost:62115",
		// TODO(ddoman): Set the default recorder address.
		Recorder: "",

		LoggingConfig: logging.Config{Level: logging.Debug},
	}
}

// Register adds sinkWrapper related flags to a FlagSet.
func (f *sinkWrapperFlags) register(fs *flag.FlagSet) {
	// flags for the test results
	fs.StringVar(&f.InvocationIDFile, "invocation-id-file", f.InvocationIDFile,
		"Path to write the generated invocation ID")
	fs.StringVar(&f.TestIDPrefix, "test-id-prefix", f.TestIDPrefix,
		"Prefix to prepepend before the test id of every test result")
	fs.Var(luciflag.StringPairs(f.TestBaseVariants), "test-base-variant",
		"Base for variant def specified for test result, of form key:value, may be set multiple times")
	fs.Var(luciflag.StringPairs(f.TestResultTags), "test-result-tag",
		"Tag to add to all test results, of form key:value, may be set multiple times")

	fs.Var(CommaCodeList(&f.CompleteInvocationExitCodes),
		"complete-invocation-exit-codes",
		"Comma-separated list of exit codes from the subprocess that mean the Invocation should be marked non-interrupted")

	// flags for the SinkServer
	fs.StringVar(&f.SinkAddress, "sink-address", f.SinkAddress,
		"TCP address for the SinkServer to listen on")
	fs.StringVar(&f.Recorder, "recorder", f.Recorder,
		"Hostname of the Recorder service that the server should upload results to")

	// flags for logs
	fs.StringVar(&f.LogFile, "log-file", f.LogFile, "the logoutput")
	f.LoggingConfig.AddFlags(fs)
}

func parseFlags() (*sinkWrapperFlags, []string, error) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	wrapperFlags := newFlags()
	wrapperFlags.register(fs)
	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, []string{}, err
	}
	return wrapperFlags, fs.Args(), nil
}

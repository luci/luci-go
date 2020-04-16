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

package wrapper

import (
	"flag"
	"strconv"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/resultdb/sink"
)

type Flags struct {
	sink     string
	recorder string

	invocation  string
	updateToken string

	testIDPrefix   string
	invocationTags strpair.Map

	completeInvocationExitCodes []int
	logFile                     string
}

func parseFlags() (Flags, error) {
	flgs := Flags{
		invocationTags: make(strpair.Map),
	}

	flag.StringVar(&flgs.sink, "sink", sink.DefaultAddr,
		"Address for SinkServer to listen on")
	flag.StringVar(&flgs.recorder, "recorder", chromeinfra.ResultDBHost,
		"Address of the Recorder service to report test results to")

	flag.StringVar(&flgs.invocation, "invocation", "",
		"The name of the invocation to append the test results to")
	flag.StringVar(&flgs.updateToken, "update-token", "",
		"The token required for the invocation to be updated")

	flag.StringVar(&flgs.testIDPrefix, "test-id-prefix", "",
		"Prefix to prepepend before the test id of every test result")
	flag.Var(luciflag.StringPairs(flgs.invocationTags), "invocation-tag",
		"Tag to add to the Invocation, of form key:value, may be set more than once")

	flag.StringVar(&flgs.logFile, "log-file", "", "File to log to")

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

	flgs.completeInvocationExitCodes = make([]int, len(rawExitCodes))
	for i, rawCode := range rawExitCodes {
		code, err := strconv.Atoi(rawCode)
		if err != nil {
			return flgs, errors.Annotate(err, "must pass integers to -complete-invocation-exit-codes").Err()
		}
		flgs.completeInvocationExitCodes[i] = code
	}

	if len(flgs.completeInvocationExitCodes) == 0 {
		flgs.completeInvocationExitCodes = []int{0}
	}

	return flgs, nil
}

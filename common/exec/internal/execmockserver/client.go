// Copyright 2023 The LUCI Authors.
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

package execmockserver

import (
	"flag"
	"fmt"
	"net/rpc"
	"os"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
)

type client struct {
	client       *rpc.Client
	invocationID uint64
}

func (c *client) getIvocationInput() *InvocationInput {
	var ret InvocationInput
	if err := c.client.Call("ExecMockServer.GetInvocationInput", c.invocationID, &ret); err != nil {
		panic(err)
	}
	return &ret
}

func (c *client) setInvocationOutput(rslt, err reflect.Value, panicStack string) {
	out := &InvocationOutput{
		InvocationID: c.invocationID,
		RunnerOutput: rslt.Interface(),
		RunnerPanic:  panicStack,
	}

	if !err.IsNil() {
		out.RunnerError = err.Interface().(error).Error()
	}

	var ignore int
	if err := c.client.Call("ExecMockServer.SetInvocationOutput", out, &ignore); err != nil {
		panic(err)
	}
}

// ClientIntercept will look for the LUCI_EXECMOCK_CTX environment variable, and, if
// found, invoke `cb` with an initialized Client as well as the input
// corresponding to this invocation.
//
// The callback will execute the mock function with the decoded input, and then
// possibly call Client.SetInvocationOutput.
//
// See go.chromium.org/luci/common/execmock
func ClientIntercept(runnerRegistry map[uint64]reflect.Value) (exitcode int, intercepted bool) {
	endpoint := os.Getenv(execServeEnvvar)
	if endpoint == "" {
		return 0, false
	}
	intercepted = true

	// We reset flag.CommandLine to make it easier to use `flag` from a runner
	// function without accidentally picking up the default flags registered by
	// `go test`.
	oldCLI := flag.CommandLine
	defer func() { flag.CommandLine = oldCLI }()
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// This is the mock subprocess invocation.

	// hide the envvar from the RunnerFunction for cleanliness purposes.
	if err := os.Unsetenv(execServeEnvvar); err != nil {
		panic(err)
	}

	tokens := strings.Split(endpoint, "|")
	if len(tokens) != 2 {
		panic(errors.Fmt("%s: expected two tokens, got %q", execServeEnvvar, endpoint))
	}
	hostname, invocationIDstr := tokens[0], tokens[1]
	invocationID, err := strconv.ParseUint(invocationIDstr, 10, 64)
	if err != nil {
		panic(err)
	}
	rpcClient, err := rpc.DialHTTP("tcp", hostname)
	if err != nil {
		panic(err)
	}

	emClient := &client{rpcClient, invocationID}
	input := emClient.getIvocationInput()
	runnerFn := runnerRegistry[input.RunnerID]
	if !runnerFn.IsValid() {
		panic(fmt.Sprintf("unknown runner id %d", input.RunnerID))
	}

	inputType := runnerFn.Type().In(0)

	// GoB will always decode this as flat, so if the runner is expecting *T or
	// v will just be type T.
	v := reflect.ValueOf(input.RunnerInput)
	if v.Type() != inputType && inputType.Kind() == reflect.Pointer {
		newV := reflect.New(v.Type())
		newV.Elem().Set(v)
		v = newV
	}

	defer func() {
		if thing := recover(); thing != nil {
			stack := string(debug.Stack())
			exitcode = 1
			if err, ok := thing.(error); ok {
				emClient.setInvocationOutput(
					reflect.New(runnerFn.Type().Out(0)).Elem(), reflect.ValueOf(err), stack)
			} else {
				emClient.setInvocationOutput(
					reflect.New(runnerFn.Type().Out(0)).Elem(), reflect.ValueOf(errors.Fmt("%s", thing)), stack)
			}
		}
	}()
	results := runnerFn.Call([]reflect.Value{v})
	emClient.setInvocationOutput(results[0], results[2], "")

	return int(results[1].Int()), true
}

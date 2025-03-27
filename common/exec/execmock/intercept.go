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

package execmock

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"

	"go.chromium.org/luci/common/exec/internal/execmockctx"
	"go.chromium.org/luci/common/exec/internal/execmockserver"
)

var server *execmockserver.Server

func startServer() {
	server = execmockserver.Start()
}

var execmockList = flag.Bool("execmock.list", false, "Lists the names of all registered runners and their ID.")

const (
	execmockRunnerIDEnv     = "EXECMOCK_RUNNER_ID"
	execmockRunnerInputEnv  = "EXECMOCK_RUNNER_INPUT"
	execmockRunnerOutputEnv = "EXECMOCK_RUNNER_OUTPUT"
)

// Intercept must be called from TestMain like:
//
//	func TestMain(m *testing.M) {
//		execmock.Intercept(false)
//		os.Exit(m.Run())
//	}
//
// If process flags have not yet been parsed, this will call flag.Parse().
//
// If strict is `true`, it requires that 100% of luci/common/exec uses consume
// a context which was prepared with execmock.Init.
//
// If strict is `false`, luci/common/exec uses with a context without any mock
// information will be passed-through to the real execution.
func Intercept(strict bool) {
	runnerMu.Lock()
	runnerRegistryMutable = false
	registry := runnerRegistry
	registryMeta := runnerRegistryMeta
	runnerMu.Unlock()

	if exitcode, intercepted := execmockserver.ClientIntercept(registry); intercepted {
		os.Exit(exitcode)
	}

	if !flag.Parsed() {
		flag.Parse()
	}

	if *execmockList {
		fmt.Println("execmock Registered Runners:")
		fmt.Println()
		fmt.Println("<ID>: Type - Registration location")
		for id, entry := range registry {
			t := entry.Type()
			meta := registryMeta[id]
			if meta.file != "" {
				fmt.Printf("  %d: Runner[%s, %s] - %s:%d\n", id, t.In(0), t.Out(0), meta.file, meta.line)
			} else {
				fmt.Printf("  %d: Runner[%s, %s]\n", id, t.In(0), t.Out(0))
			}
		}
		fmt.Println()
		fmt.Printf("To execute a single runner, set the envvar $%s to <ID>.\n", execmockRunnerIDEnv)
		fmt.Printf("To provide input, write it in JSON to a file and set the file in $%s.\n", execmockRunnerInputEnv)
		fmt.Printf("To see output, set the output file in $%s.\n", execmockRunnerOutputEnv)
		fmt.Printf("After preparing the environment, run `go test`.")
		os.Exit(0)
	}

	if idStr := os.Getenv(execmockRunnerIDEnv); idStr != "" {
		inFilePath := os.Getenv(execmockRunnerInputEnv)
		outFilePath := os.Getenv(execmockRunnerOutputEnv)

		// prune all execmock envars from Env
		os.Unsetenv(execmockRunnerIDEnv)
		os.Unsetenv(execmockRunnerInputEnv)
		os.Unsetenv(execmockRunnerOutputEnv)

		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			log.Fatalf("execmock: $%s: not an id: %s", execmockRunnerIDEnv, err)
		}

		fn, ok := registry[id]
		if !ok {
			log.Fatalf("execmock: $%s: unknown Runner id: %d", execmockRunnerIDEnv, id)
		}

		inData := reflect.New(fn.Type().In(0)).Elem()
		if inFilePath != "" {
			inFile, err := os.Open(inFilePath)
			if err != nil {
				log.Fatalf("opening execmock.input: %s", err)
			}
			if err = json.NewDecoder(inFile).Decode(inData.Addr().Interface()); err != nil {
				log.Fatalf("decoding execmock.input: %s", err)
			}
		}

		results := fn.Call([]reflect.Value{inData})

		if outFilePath != "" {
			outFile, err := os.Create(outFilePath)
			if err != nil {
				log.Fatalf("opening execmock.output: %s", err)
			}
			toEnc := struct {
				Error string
				Data  any
			}{Data: results[0].Interface()}
			if errVal := results[2]; !errVal.IsNil() {
				toEnc.Error = errVal.Interface().(error).Error()
			}
			if err = json.NewEncoder(outFile).Encode(toEnc); err != nil {
				log.Fatalf("encoding execmock.output: %s", err)
			}
		}

		os.Exit(int(results[1].Int()))
	}

	// This is the real `go test` invocation.
	execmockctx.EnableMockingForThisProcess(getMocker, strict)
	startServer()
}

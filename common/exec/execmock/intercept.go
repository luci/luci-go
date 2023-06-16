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
	"os"

	"go.chromium.org/luci/common/exec/internal/execmockctx"
	"go.chromium.org/luci/common/exec/internal/execmockserver"
)

var server *execmockserver.Server

func startServer() {
	server = execmockserver.Start()
}

// Intercept must be called from TestMain like:
//
//	func TestMain(m *testing.M) {
//	  execmock.Intercept()
//	  // Call flag.Parse() here if TestMain uses flags.
//	  os.Exit(m.Run())
//	}
func Intercept() {
	runnerMu.Lock()
	runnerRegistryMutable = false
	registry := runnerRegistry
	runnerMu.Unlock()

	if exitcode, intercepted := execmockserver.ClientIntercept(registry); intercepted {
		os.Exit(exitcode)
	}

	// This is the real `go test` invocation.
	execmockctx.EnableMockingForThisProcess(getMocker)
	startServer()
}

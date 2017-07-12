// Copyright 2015 The LUCI Authors.
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

package annotation

import (
	"os"

	"github.com/luci/luci-go/common/system/environ"
)

// Execution describes the high-level execution metadata.
type Execution struct {
	Name    string
	Command []string
	Dir     string
	Env     map[string]string
}

// ProbeExecution loads Execution parameters by probing the current runtime
// environment.
func ProbeExecution(argv, env []string, cwd string) *Execution {
	if env == nil {
		env = os.Environ()
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return probeExecutionImpl(argv, env, cwd)
}

func probeExecutionImpl(argv []string, env []string, cwd string) *Execution {
	e := &Execution{
		Dir: cwd,

		// Unique-ify the environment variables.
		Env: environ.New(env).Map(),
	}
	e.Command = make([]string, len(argv))
	copy(e.Command, argv)

	return e
}

// Copyright 2017 The LUCI Authors.
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

package application

import (
	"context"
	"os/exec"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
)

// Exec runs the specified Python command.
//
// Once the process launches, Context cancellation will not have an impact.
//
// If an error occurs during execution, it will be returned here. Otherwise,
// Exec will not return, and this process will exit with the return code of the
// executed process.
//
// The implementation of Exec is platform-specific.
func cmdExec(c context.Context, cmd *exec.Cmd) error {
	logging.Debugf(c, "Exec command: %#v", cmd)
	return execImpl(c, cmd.Args, environ.New(cmd.Env), cmd.Dir)
}

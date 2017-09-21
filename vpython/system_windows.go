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

package vpython

import (
	"os"

	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"golang.org/x/net/context"
)

// systemSpecificLaunch launches the process described by "cmd" while ensuring
// that the VirtualEnv lock is held throughout its duration (best effort).
//
// On Windows, we don't forward signals. Forwarding signals on Windows is
// nuanced. For now, we won't, since sending them via Python is similarly
// nuanced and not commonly done.
//
// For more discussion, see:
// https://github.com/golang/go/issues/6720
//
// On Windows, we launch it as a child process and interpret any signal that we
// receive as terminal, cancelling the child.
func systemSpecificLaunch(c context.Context, ve *venv.Env, argv []string, env environ.Env, dir string) error {
	c, cancelFunc := context.WithCancel(c)
	defer cancelFunc()

	cmd := ve.Interpreter().IsolatedCommand(c, argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env.Sorted()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	logging.Debugf(c, "Python subprocess has terminated: %v", err)
	if err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}

func runSystemCommand(context.Context, string, string, environ.Env) (int, bool) { return 0, false }

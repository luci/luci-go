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
	"os/signal"
	"syscall"

	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"golang.org/x/net/context"
)

// systemSpecificLaunch launches the process described by "cmd" while ensuring
// that the VirtualEnv lock is held throughout its duration (best effort).
//
// Only the Args, Env, and Dir fields of "cmd" are examined.
//
// On Windows, we launch it as a child process and interpret any signal that we
// receive as terminal, cancelling the child.
func systemSpecificLaunch(c context.Context, ve *venv.Env, args []string, env environ.Env, dir string) error {
	c, cancelFunc := context.WithCancel(c)

	cmd := ve.Interpreter().IsolatedCommand(c, args...)
	cmd.Dir = dir
	cmd.Env = env.Sorted()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	signalC := make(chan os.Signal, 1)
	signalDoneC := make(chan struct{})
	signal.Notify(signalC, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)
	defer func() {
		signal.Stop(signalC)

		close(signalC)
		<-signalDoneC
	}()

	if err := cmd.Start(); err != nil {
		return errors.Annotate(err, "failed to start process").Err()
	}

	logging.Fields{
		"pid": cmd.Process.Pid,
	}.Debugf(c, "Python subprocess has started!")

	// Start our signal forwarding goroutine, now that the process is running.
	go func() {
		defer func() {
			close(signalDoneC)
		}()

		for sig := range signalC {
			logging.Debugf(c, "Received signal %v, terminating child...", sig)
			cancelFunc()
		}
	}()

	err := cmd.Wait()
	logging.Debugf(c, "Python subprocess has terminated: %v", err)
	if err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}

func runSystemCommand(context.Context, string, string, environ.Env) (int, bool) { return 0, false }

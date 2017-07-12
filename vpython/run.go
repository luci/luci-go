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
	"os/exec"
	"os/signal"
	"strings"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/system/environ"
	"github.com/luci/luci-go/vpython/venv"

	"golang.org/x/net/context"
)

const (
	// EnvironmentStampPathENV is the exported environment variable for the
	// environment stamp path.
	//
	// This is added to the bootstrap environment used by Run to allow subprocess
	// "vpython" invocations to automatically inherit the same environment.
	EnvironmentStampPathENV = "VPYTHON_VENV_ENV_STAMP_PATH"
)

// Run sets up a Python VirtualEnv and executes the supplied Options.
//
// Run returns nil if if the Python environment was successfully set-up and the
// Python interpreter was successfully run with a zero return code. If the
// Python interpreter returns a non-zero return code, a PythonError (potentially
// wrapped) will be returned.
//
// A generalized return code to return for an error value can be obtained via
// ReturnCode.
//
// Run consists of:
//
//	- Identify the target Python script to run (if there is one).
//	- Identifying the Python interpreter to use.
//	- Composing the environment specification.
//	- Constructing the virtual environment (download, install).
//	- Execute the Python process with the supplied arguments.
//
// The Python subprocess is bound to the lifetime of ctx, and will be terminated
// if ctx is cancelled.
func Run(c context.Context, opts Options) error {
	// Resolve our Options.
	if err := opts.resolve(c); err != nil {
		return errors.Annotate(err, "could not resolve options").Err()
	}

	// Create a local cancellation option (signal handling).
	c, cancelFunc := context.WithCancel(c)
	defer cancelFunc()

	// Create our virtual environment root directory.
	err := venv.With(c, opts.EnvConfig, opts.WaitForEnv, func(c context.Context, ve *venv.Env) error {
		// Build the augmented environment variables.
		e := opts.Environ
		if e.Len() == 0 {
			// If no environment was supplied, use the system environment.
			e = environ.System()
		}

		// Remove PYTHONPATH and PYTHONHOME from the environment. This prevents them
		// from being propagated to delegate processes (e.g., "vpython" script calls
		// Python script, the "vpython" one uses the Interpreter's IsolatedCommand
		// to isolate the initial run, but the delegate command blindly uses the
		// environment that it's provided).
		//
		// Also set PYTHONNOUSERSITE, which prevents a user's "site" configuration
		// from influencing Python startup. The system "site" should already be
		// ignored b/c we're using the VirtualEnv Python interpreter.
		e.Remove("PYTHONPATH")
		e.Remove("PYTHONHOME")
		e.Set("PYTHONNOUSERSITE", "1")

		e.Set("VIRTUAL_ENV", ve.Root) // Set by VirtualEnv script.
		if ve.EnvironmentStampPath != "" {
			e.Set(EnvironmentStampPathENV, ve.EnvironmentStampPath)
		}

		// Prefix PATH with the VirtualEnv "bin" directory.
		prefixPATH(e, ve.BinDir)

		// Run our bootstrapped Python command.
		cmd := ve.Interpreter().IsolatedCommand(c, opts.Args...)
		cmd.Dir = opts.WorkDir
		cmd.Env = e.Sorted()
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		logging.Debugf(c, "Running Python command: %s\nWorkDir: %s\nEnv: %s", cmd.Args, cmd.Dir, cmd.Env)

		// Output the Python command being executed.
		if err := runAndForwardSignals(c, cmd, cancelFunc); err != nil {
			return errors.Annotate(err, "failed to execute bootstrapped Python").Err()
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}

func runAndForwardSignals(c context.Context, cmd *exec.Cmd, cancelFunc context.CancelFunc) error {
	signalC := make(chan os.Signal, 1)
	signalDoneC := make(chan struct{})
	signal.Notify(signalC, forwardedSignals...)
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
			logging.Debugf(c, "Forwarding signal: %v", sig)
			if err := cmd.Process.Signal(sig); err != nil {
				logging.Fields{
					logging.ErrorKey: err,
					"signal":         sig,
				}.Errorf(c, "Failed to forward signal; terminating immediately.")
				cancelFunc()
			}
		}
	}()

	err := cmd.Wait()
	logging.Debugf(c, "Python subprocess has terminated: %v", err)
	if err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}

func prefixPATH(env environ.Env, components ...string) {
	if len(components) == 0 {
		return
	}

	// Clone "components" so we don't mutate our caller's array.
	components = append([]string(nil), components...)

	// If there is a current PATH (likely), add that to the end.
	cur, _ := env.Get("PATH")
	if len(cur) > 0 {
		components = append(components, cur)
	}

	env.Set("PATH", strings.Join(components, string(os.PathListSeparator)))
}

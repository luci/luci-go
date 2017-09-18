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

// +build darwin linux freebsd netbsd openbsd android

package vpython

import (
	"io/ioutil"
	"os"
	"runtime"
	"syscall"
	"time"

	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"golang.org/x/net/context"
)

const (
	applicationCommandMonitorProcess = "POSIXMonitorProcess"
)

var isDarwin = runtime.GOOS == "darwin"

type monitoringProcessConfig struct {
	PID      int           `json:"pid"`
	LockPath string        `json:"lock_path"`
	Level    logging.Level `json:"log_level"`
}

// systemSpecificLaunch launches the process described by "cmd" while ensuring
// that the VirtualEnv lock is held throughout its duration (best effort).
//
// Only the Args, Env, and Dir fields of "cmd" are examined.
//
// On Linux/Mac, we use "execve" to *become* the target process. We need to
// continue to hold the lock for that process, though.
//
// One option is to pass the file handle to the process. However, this is
// invasive, and fundamentally changes the process in a non-transparent way.
//
// Instead, we start a monitoring process. This process will be born holding the
// lock. It will periodically check to see if the Python process group is alive
// and, if not, self-terminate.
func systemSpecificLaunch(c context.Context, ve *venv.Env, args []string, env environ.Env, dir string) error {
	// Create a signalling pipe.
	//
	// Make sure that we clean up after it even if we fail to complete the
	// launch.
	rp, wp, err := os.Pipe()
	if err != nil {
		return errors.Annotate(err, "could not create signalling pipe").Err()
	}
	defer rp.Close()
	defer func() {
		if wp != nil {
			wp.Close()
		}
	}()

	if err := startMonitoringProcess(c, ve, wp, env); err != nil {
		return errors.Annotate(err, "could not spawn monitor process").Err()
	}

	// Close our "write" pipe. Our monitor process owns the only copy now.
	if err := wp.Close(); err != nil {
		return errors.Annotate(err, "failed to close parent write pipe").Err()
	}
	wp = nil // Don't close on defer.

	// Wait for our monitoring process to hold the environment lock.
	if _, err := ioutil.ReadAll(rp); err != nil {
		return errors.Annotate(err, "failed to wait for monitoring process signal").Err()
	}
	logging.Debugf(c, "Signalled by monitoring process, launching...")

	// Change directory.
	if dir != "" {
		if err := os.Chdir(dir); err != nil {
			return errors.Annotate(err, "failed to chdir to %q", dir).Err()
		}
	}

	// This is the original process. Become Python.
	if err := syscall.Exec(args[0], args, env.Sorted()); err != nil {
		return errors.Annotate(err, "failed to execve %q", args[0]).Err()
	}
	return nil
}

func startMonitoringProcess(c context.Context, ve *venv.Env, wp *os.File, env environ.Env) error {
	// Construct our monitoring process config and set it in our monitoring
	// subprocess' environment.
	cfg := monitoringProcessConfig{
		PID:      os.Getpid(),
		LockPath: ve.LockPath,
		Level:    logging.GetLevel(c),
	}

	monCmd, err := createCommand(c, applicationCommandMonitorProcess, &cfg, env)
	if err != nil {
		return err
	}
	monCmd.ExtraFiles = []*os.File{
		// 3: Write pipe to close in order to signal readiness.
		wp,
	}
	monCmd.SysProcAttr = &syscall.SysProcAttr{
		// Create a new process group for the monitor process.
		Setpgid: true,
	}
	if err := monCmd.Start(); err != nil {
		return errors.Annotate(err, "could not start monitoring process").Err()
	}

	return nil
}

func runSystemCommand(c context.Context, name, val string, env environ.Env) (int, bool) {
	switch name {
	case applicationCommandMonitorProcess:
		return runMonitor(c, val), true

	default:
		return 0, false
	}
}

func runMonitor(c context.Context, encodedConfig string) int {
	// Get our write pipe handle.
	wp := os.NewFile(3, "")
	defer func() {
		if wp != nil {
			wp.Close()
		}
	}()

	var cfg monitoringProcessConfig
	if err := envValueDecode(encodedConfig, &cfg); err != nil {
		logging.WithError(err).Errorf(c, "Failed to load config.")
		return 1
	}

	// Set our logging level to match our spawning process.
	c = logging.SetLevel(c, cfg.Level)

	// Acquire a compatible shared lock for our environment.
	err := venv.WithSharedLock(cfg.LockPath, func() error {
		// Notify that we obtained the shared lock by closing "wp".
		if err := wp.Close(); err != nil {
			return errors.Annotate(err, "failed to close signal pipe").Err()
		}
		wp = nil // Don't close on defer.

		const pidCheckFrequency = time.Second
		logging.Debugf(c, "Acquired environment shared lock, polling with frequency %s...", pidCheckFrequency)
		for {
			if err := syscall.Kill(cfg.PID, 0); err != nil {
				logging.WithError(err).Infof(c, "Failed to kill parent process group %d.", cfg.PID)
				return nil
			}

			time.Sleep(pidCheckFrequency)
		}
	})
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to acquire environment lock for: %q", cfg.LockPath)
		return 1
	}

	logging.Debugf(c, "Monitoring process has released its shared lock and is terminating.")
	return 0
}

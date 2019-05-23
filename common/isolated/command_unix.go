// Copyright 2019 The LUCI Authors.
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

package isolated

import (
	"context"
	"log"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
)

// runCommand runs the command.
func runCommand(ctx context.Context, command []string, cwd string, env environ.Env, hardTimeout time.Duration, gracePeriod time.Duration) (int, bool, error) {
	localctx, cancel := context.WithTimeout(ctx, hardTimeout)
	defer cancel()

	killch := time.After(hardTimeout + gracePeriod)

	cmd := exec.Command(command[0], command[1:]...)
	cmd.Env = env.Sorted()
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return 0, false, errors.Annotate(err, "failed to start command: %s", command).Err()
	}

	var wg errgroup.Group

	done := make(chan struct{})

	timeout := false

	wg.Go(func() error {
		// wait sigterm timeout
		select {
		case <-localctx.Done():
			timeout = true
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				return errors.Annotate(err, "failed to send SIGTERM").Err()
			}
		case <-done:
			return nil
		}

		clean := func() error {
			if err := cmd.Process.Kill(); err != nil {
				return errors.Annotate(err, "failed to send SIGKILL").Err()
			}
			return nil
		}

		// wait kill timeout
		select {
		case <-killch:
			return clean()
		case <-ctx.Done():
			if err := clean(); err != nil {
				return err
			}
			return ctx.Err()
		case <-done:
			return nil
		}
	})

	err := cmd.Wait()
	close(done)

	exitCode := cmd.ProcessState.ExitCode()
	log.Print(err)
	if err, ok := err.(*exec.ExitError); ok {
		exitCode = err.ExitCode()
	} else if err != nil {
		return 0, false, errors.Annotate(err, "failed to wait command").Err()
	}

	return exitCode, timeout, wg.Wait()
}

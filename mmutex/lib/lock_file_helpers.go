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

package lib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/danjacques/gofslock/fslock"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
)

// LockFileEnvVariable specifies the directory of the lock file.
const LockFileEnvVariable = "MMUTEX_LOCK_DIR"

// LockFileName specifies the name of the lock file within $MMUTEX_LOCK_DIR.
const LockFileName = "mmutex.lock"

// DrainFileName specifies the name of the drain file within $MMUTEX_LOCK_DIR.
const DrainFileName = "mmutex.drain"

// DefaultCommandTimeout is the total amount of time, including lock acquisition
// and command runtime, allotted to running a command through mmutex.
const DefaultCommandTimeout = 2 * time.Hour

// lockAcquisitionAttempts is the number of times that the blocker should attempt to
// acquire the lock. This is approximate, especially when making a high number of
// attempts over a short period of time.
const lockAcquisitionAttempts = 100

// defaultLockPollingInterval is the polling interval used if no command
// timeout is specified.
const defaultLockPollingInterval = time.Millisecond

// computeMutexPaths returns the lock and drain file paths based on the environment,
// or empty strings if no lock files should be used.
func computeMutexPaths(env subcommands.Env) (lockFilePath string, drainFilePath string, err error) {
	envVar := env[LockFileEnvVariable]
	if !envVar.Exists {
		return "", "", nil
	}

	lockFileDir := envVar.Value
	if !filepath.IsAbs(lockFileDir) {
		return "", "", errors.Fmt("Lock file directory %s must be an absolute path", lockFileDir)
	}

	if _, err := os.Stat(lockFileDir); os.IsNotExist(err) {
		fmt.Printf("Lock file directory %s does not exist, mmutex acting as a passthrough.\n", lockFileDir)
		return "", "", nil
	}

	return filepath.Join(lockFileDir, LockFileName), filepath.Join(lockFileDir, DrainFileName), nil
}

func createLockBlocker(ctx context.Context) fslock.Blocker {
	pollingInterval := defaultLockPollingInterval
	if deadline, ok := ctx.Deadline(); ok {
		pollingInterval = clock.Until(ctx, deadline) / (lockAcquisitionAttempts - 1)
	}

	// crbug.com/1038136
	// The default timeout is 120 minutes and the lock acquisition attempts is 100.
	// With this setting, the polling interval is 72 seconds which is too long for
	// lock acquisition. Setting max interval to be 1 second.
	if pollingInterval > time.Second {
		pollingInterval = time.Second
	}

	return func() error {
		if clock.Sleep(ctx, pollingInterval).Err != nil {
			return fslock.ErrLockHeld
		}

		// Returning nil signals that the lock should be retried.
		return nil
	}
}

// blockWhileFileExists blocks until the file located at path no longer exists.
// For convenience, this method reuses the Blocker interface exposed by fslock
// and used elsewhere in this package.
func blockWhileFileExists(path string, blocker fslock.Blocker) error {
	for {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		} else if err != nil {
			return errors.Fmt("failed to stat %s: %w", path, err)
		}

		if err := blocker(); err != nil {
			return errors.New("timed out waiting for drain file to disappear")
		}
	}

	return nil
}

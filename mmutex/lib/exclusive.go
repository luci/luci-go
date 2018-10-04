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
	"os"

	"github.com/danjacques/gofslock/fslock"
	"github.com/maruel/subcommands"
)

// RunExclusive runs the command with the specified context and environment while
// holding an exclusive mmutex lock.
func RunExclusive(ctx context.Context, env subcommands.Env, command func(context.Context) error) error {
	lockFilePath, drainFilePath, err := computeMutexPaths(env)
	if err != nil {
		return err
	}

	if len(lockFilePath) == 0 {
		return command(ctx)
	}

	file, err := os.OpenFile(drainFilePath, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}

	// Remove the drain file in case the lock can never be acquired.
	defer os.Remove(drainFilePath)

	blocker := createLockBlocker(ctx)
	return fslock.WithBlocking(lockFilePath, blocker, func() error {
		// Remove the drain file immediately after acquiring the lock in order
		// to decrease the likelihood that a crash occurs, leaving the drain
		// file sitting around indefinitely.
		if err := os.Remove(drainFilePath); err != nil {
			return err
		}

		return command(ctx)
	})
}

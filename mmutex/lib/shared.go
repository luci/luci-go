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

	"github.com/danjacques/gofslock/fslock"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/logging"
)

// RunShared runs the command with the specified context and environment while
// holding a shared mmutex lock.
func RunShared(ctx context.Context, env subcommands.Env, command func(context.Context) error) error {
	lockFilePath, drainFilePath, err := computeMutexPaths(env)
	if err != nil {
		return err
	}

	logging.Infof(ctx, "[mmutex][shared] LockFilePath: %s, DrainFilePath: %s.", lockFilePath, drainFilePath)
	if len(lockFilePath) == 0 {
		return command(ctx)
	}

	blocker := createLockBlocker(ctx)

	// Use the same retry mechanism for checking if the drain file still exists
	// as we use to request the file lock.
	if err = blockWhileFileExists(drainFilePath, blocker); err != nil {
		return err
	}

	return fslock.WithSharedBlocking(lockFilePath, blocker, func() error {
		logging.Infof(ctx, "[mmutex][shared] Lock acquired.")
		return command(ctx)
	})
}

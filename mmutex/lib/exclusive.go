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
	"time"

	"github.com/danjacques/gofslock/fslock"
	"github.com/maruel/subcommands"
)

// RunExclusive runs the command with the specified environment while holding an
// exclusive mmutex lock.
func RunExclusive(env subcommands.Env, command func() error) error {
	lockFilePath, err := computeLockFilePath(env)
	if err != nil {
		return err
	}

	if len(lockFilePath) == 0 {
		return command()
	}

	blocker := CreateBlockerUntil(time.Now().Add(fslockTimeout), fslockPollingInterval)
	return fslock.WithBlocking(lockFilePath, blocker, func() error {
		return command()
	})
}

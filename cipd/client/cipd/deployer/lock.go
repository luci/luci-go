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

package deployer

// Note: FS locking functionality is moved into a separate file for easier
// replacement if cipd codebase is vendored.

import (
	"github.com/danjacques/gofslock/fslock"
)

// lockFS grabs an exclusive file system lock and returns a callback that
// releases it.
//
// If the lock is already taken, calls waiter(), assuming it will sleep a bit.
//
// Retries either until the lock is successfully acquired or waiter() returns
// an error.
func lockFS(path string, waiter func() error) (unlock func() error, err error) {
	l := fslock.L{
		Path:  path,
		Block: fslock.Blocker(waiter),
	}
	handle, err := l.Lock()
	if err != nil {
		return nil, err
	}
	return handle.Unlock, nil
}

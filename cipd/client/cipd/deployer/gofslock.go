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
// replacement/disablement if CIPD codebase is partially vendored.

import (
	"github.com/danjacques/gofslock/fslock"
)

func init() {
	if lockFS != nil {
		panic("lockFS is already initialized")
	}
	// See deployer.go for where lockFS is defined and used.
	lockFS = func(path string, waiter func() error) (unlock func() error, err error) {
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
}

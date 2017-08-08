// Copyright 2016 The LUCI Authors.
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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

var counter int32

// AtomicWriteFile atomically replaces the content of a given file.
//
// It will retry a bunch of times on permission errors on Windows, where reader
// may lock the file.
func AtomicWriteFile(c context.Context, path string, body []byte, perm os.FileMode) error {
	// Write the body to some temp file in the same directory.
	tmp := fmt.Sprintf(
		"%s.%d_%d_%d.tmp", path, os.Getpid(),
		atomic.AddInt32(&counter, 1), time.Now().UnixNano())
	if err := ioutil.WriteFile(tmp, body, perm); err != nil {
		return err
	}

	// Try to replace the target file a bunch of times, retrying "Access denied"
	// errors. They happen on Windows if file is locked by some other process.
	// We assume other processes do not lock token file for too long.
	c, _ = context.WithTimeout(c, 10*time.Second)
	var err error

loop:
	for {
		switch err = atomicRename(tmp, path); {
		case err == nil:
			return nil // success!
		case !os.IsPermission(err):
			break loop // some unretriable fatal error
		default: // permission error
			logging.Warningf(c, "Failed to replace %q - %s", path, err)
			if tr := clock.Sleep(c, 500*time.Millisecond); tr.Incomplete() {
				break loop // timeout, giving up
			}
		}
	}

	// Best effort in cleaning up the garbage.
	os.Remove(tmp)
	return err
}

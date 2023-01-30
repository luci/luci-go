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

package streamclient

import (
	"runtime"
	"time"
)

func init() {
	// The numbers here are somewhat arbitrary: On my mac laptop the entire test
	// suite takes .1s to complete in normal mode, and 2s to complete in race
	// mode.
	//
	// On my windows 10 vm, the entire suite takes .5s to complete in normal mode
	// and 2s (yep, same speed) to complete in race mode.
	//
	// Because of the apparent overhead, we multiply the fuse by 10, giving us
	// a normal 10s fuse per-test-case and race 100s fuse per-test-case (not
	// suite!). This should be more than enough, but is still a lot better than
	// the default 10m test timeout.
	if runtime.GOOS == "windows" {
		timebombFuse *= 10
	}
}

// timebomb forcibly crashes the test executable after timebombFuse time.
//
// Fuse times (x5 on windows):
//   - 1 second in non-race mode
//   - 10 seconds in race mode
//
// use like `defer timebomb()()`
func timebomb() func() {
	diffuseTimebomb := make(chan struct{})
	timebombDiffused := make(chan struct{})
	go func() {
		defer close(timebombDiffused)
		select {
		case <-time.After(timebombFuse):
			panic("runWireProtocolTest took too long.")
		case <-diffuseTimebomb:
		}
	}()
	return func() {
		close(diffuseTimebomb)
		<-timebombDiffused // explicitly sync with the completion of the goroutine.
	}
}

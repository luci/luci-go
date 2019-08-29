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
	if runtime.GOOS == "windows" {
		timebombFuse *= 2
	}
}

// timebomb forcibly crashes the test executable after timebombFuse time.
//
// Fuse times (x2 on windows):
//   * 1 second in non-race mode
//   * 10 seconds in race mode
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

// Copyright 2015 The LUCI Authors.
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

package paniccatcher

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCatch(t *testing.T) {
	Convey(`Catch will suppress a panic.`, t, func() {
		var pv *Panic
		So(func() {
			defer Catch(func(p *Panic) {
				pv = p
			})
			panic("Everybody panic!")
		}, ShouldNotPanic)
		So(pv, ShouldNotBeNil)
		So(pv.Reason, ShouldEqual, "Everybody panic!")
		So(pv.Stack, ShouldContainSubstring, "TestCatch")
	})
	Convey(`Body does not run with no panic`, t, func() {
		didRun := false
		func() {
			defer Catch(func(*Panic) {
				didRun = true
			})
		}()
		So(didRun, ShouldBeFalse)
	})
}

// Example is a very simple example of how to use Catch to recover from a panic
// and log its stack trace.
func Example() {
	Do(func() {
		fmt.Println("Doing something...")
		panic("Something wrong happened!")
	}, func(p *Panic) {
		fmt.Println("Caught a panic:", p.Reason)
	})
	// Output: Doing something...
	// Caught a panic: Something wrong happened!
}

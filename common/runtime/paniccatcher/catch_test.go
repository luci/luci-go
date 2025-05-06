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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCatch(t *testing.T) {
	t.Run(`Catch will suppress a panic.`, func(t *testing.T) {
		var pv *Panic
		func() {
			defer Catch(func(p *Panic) {
				pv = p
			})
			panic("Everybody panic!")
		}() // should not panic
		if pv == nil {
			t.Fatalf("pv == nil (should not be)")
		}
		assert.That(t, pv.Reason.(string), should.Match("Everybody panic!"))
		if !strings.Contains(pv.Stack, "TestCatch") {
			t.Fatalf("pv.Stack does not contain TestCatch: %q", pv.Stack)
		}
	})
	t.Run(`Body does not run with no panic`, func(t *testing.T) {
		didRun := false
		func() {
			defer Catch(func(*Panic) {
				didRun = true
			})
		}()
		if didRun {
			t.Fatal("body ran somehow?")
		}
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

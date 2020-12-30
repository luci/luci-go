// Copyright 2020 The LUCI Authors.
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

package errors

import (
	"fmt"
	"runtime/debug"
	"strings"
)

func CrashingFunction() {
	panic(nil)
}

func ExampleIsPanicking() {
	A := func(crash bool) {
		defer func() {
			if IsPanicking(0) {
				fmt.Println("PANIK!")
			} else {
				fmt.Println("kalm")
			}
		}()
		if crash {
			fmt.Println("about to boom")
			CrashingFunction()
		} else {
			fmt.Println("smooth sailing")
		}
	}

	defer func() {
		// Make sure IsPanicking didn't do a `recover()`, which would goof up the
		// stack.
		stack := string(debug.Stack())
		if !strings.Contains(stack, "CrashingFunction") {
			fmt.Println("stack trace doesn't originate from CrashingFunction")
		} else {
			fmt.Println("stack trace originates from CrashingFunction")
		}
	}()

	if IsPanicking(0) {
		fmt.Println("cannot be panicing when not in defer'd function.")
	}

	A(false)
	fmt.Println("first pass success")
	A(true)

	// Output:
	// smooth sailing
	// kalm
	// first pass success
	// about to boom
	// PANIK!
	// stack trace originates from CrashingFunction
}

// Copyright 2024 The LUCI Authors.
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

// Package check contains two functions, which allow you to make fluent truth
// comparisons which t.Fail a test.
//
// This is the preferred form for tests which need to check many conditions
// without necessarially failing the entire test on the first failed condition.
//
// Example:
//
//	check.That(t, 10, should.Equal(20))
//	check.Loosely(t, myCustomInt(10), should.Equal(20))
//
// The functions in check return a boolean value, and can be used to gate
// additional checks or asserts:
//
//	if check.That(t, 10, should.Equal(20)) {
//	  // additional logic/conditions/subtests if 10 == 20
//	}
//
// This package has a sibling package `assert` which instead does t.FailNow,
// causing tests to immediately halt on the first error.
package check

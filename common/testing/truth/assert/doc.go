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

// Package assert contains two functions, which allow you to make fluent truth
// comparisons which t.FailNow a test.
//
// Example:
//
//	assert.That(t, 10, should.Equal(20))
//	assert.Loosely(t, myCustomInt(10), should.Equal(20))
//
// In the example above, the test case would halt immediately after the first
// `assert.That`, because 10 does not equal 20, and `assert.That` will call
// t.FailNow().
//
// This package has a sibling package `check` which instead does t.Fail,
// allowing tests to make multiple checks without halting.
package assert

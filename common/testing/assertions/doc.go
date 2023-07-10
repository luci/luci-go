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

// Package assertions is designed to be a collection of `.` importable, goconvey
// compatible testing assertions, in the style of
// "github.com/smarty/assertions".
//
// Due to a bug/feature in goconvey[1], files in this package end in
// `_tests.go`.
//
// This is a signal to goconvey's internal stack traversal logic (used to print
// helpful assertion messages) that the assertions in this package should be
// considered 'testing code' and not 'tested code', and so should be skipped
// over when searching for the first stack frame containing the code under test.
//
// [1]: https://github.com/smartystreets/goconvey/pull/316
package assertions

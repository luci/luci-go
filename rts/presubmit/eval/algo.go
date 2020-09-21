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

package eval

// Algorithm accepts a list of changed files and test description and
// decides whether to run it.
type Algorithm func(*Input) (*Output, error)

// Input is input to an RTS Algorithm.
type Input struct {
	// ChangedFiles is a list of files changed in a CL.
	ChangedFiles []string `json:"changedFiles"`

	// The algorithm needs to decide whether to run this test.
	// For Chromium, TestID is a ResultDB TestID.
	Test *Test
}

// Test describes a test.
type Test struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
}

// Output is the output of an RTS algorithm.
type Output struct {
	// ShouldRun is true if the test should run as a part of the suite.
	ShouldRun bool
}

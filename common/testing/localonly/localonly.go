// Copyright 2025 The LUCI Authors.
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

package localonly

import (
	"log"
	"os"
	"regexp"
	"testing"
)

var bugPattern = regexp.MustCompile(`b/[0-9]+`)

// Because skips a test in CI and writes a reference to the bug in the logs.
//
// This function is a kinder alternative to t.Skip(...) for tests that work okay locally
// but are breaking for various reasons in CI.
//
// Please do not use this function as an excuse to write bad tests.
//
// In a perfect world, this function would never be used.
func Because(t testing.TB, bug string) {
	if !bugPattern.MatchString(bug) {
		log.Fatal(`Local only tests are technical debt. You must include a valid bug number.`)
	}
	if isRunningInCI() {
		t.Skipf("skipping test in CI due to %s", bug)
	} else {
		t.Logf("test is marked local-only due to %s", bug)
	}
}

// Function isRunningInCI guesses whether we are running in a CI.
func isRunningInCI() bool {
	switch {
	case os.Getenv("CI") != "":
		return true
	case os.Getenv("SWARMING_HEADLESS") != "":
		return true
	case os.Getenv("CHROME_HEADLESS") != "":
		return true
	}
	return false
}

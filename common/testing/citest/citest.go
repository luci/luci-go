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

// Package citest inspects the environment and determines whether we are
// running in a CI environment or an environment where the user has specifically
// requested integration tests.
package citest

import (
	"os"
	"regexp"
	"testing"
)

var projectIntegrationTestPattern = regexp.MustCompile(`[A-Z][A-Z_]*[A-Z]_INTEGRATION_TESTS`)
var bugPattern = regexp.MustCompile(`b/[0-9]+`)

// IsRunningInCI guesses whether we are running in CI.
//
// TODO(gregorynisbet): Make this private again after we know
// how the fleet console is going to use this information.
// Exposing this function at all is abstraction-breaking, but
// it is still a good idea to use the same heuristic everywhere.
func IsRunningInCI() bool {
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

// wantIntegrationTests returns true if we want integraion tests based on the environment.
func wantIntegrationTests(t testing.TB, envvar string) bool {
	t.Helper()
	if envvar == "" {
		t.Fatal(`envvar in argument to wantIntegrationTests cannot be empty`)
	}
	if envvar == "INTEGRATION_TESTS" {
		t.Fatal(`test must be associated to specific project. Use e.g. "SWARMING_INTEGRATION_TESTS" not "INTEGRATION_TESTS"`)
	}
	if !projectIntegrationTestPattern.MatchString(envvar) {
		t.Fatalf(`envvar %q must be all caps, joined with underscores and end with _INTEGRATION_TESTS, like SWARMING_INTEGRATION_TESTS or DEVICE_MANAGER_INTEGRATION_TESTS`, envvar)
	}
	switch {
	case os.Getenv(envvar) != "":
		return true
	// NOTE: This check is not redundant. envvar, the variable, can't be INTEGRATION_TESTS because every test
	//       must be associated with a project, for example SWARMING_INTEGRATION_TESTS is okay as a value of envvar.
	//
	//       However, it is nevertheless possible for the INTEGRATION_TESTS environment variable to be set. When that happens,
	//       it means that we want every integration test to run, regardless of which specific project it is associated with.
	case os.Getenv("INTEGRATION_TESTS") != "":
		return true
	}
	return false
}

// IntegrationTest skips a test in a project unless we set INTEGRATION_TESTS or PROJECT_NAME_INTEGRATION_TESTS
func IntegrationTest(t testing.TB, envvar string) {
	t.Helper()
	if wantIntegrationTests(t, envvar) {
		return
	}
	t.Skipf("skipping test because neither %q or %q were set", envvar, "INTEGRATION_TESTS")
}

// LocalOnlyBecause skips a test in CI and writes a reference to the bug in the logs.
//
// This function is a kinder alternative to t.Skip(...) for tests that work okay locally
// but are breaking for various reasons in CI.
//
// Please do not use this function as an excuse to write bad tests.
//
// In a perfect world, this function would never be used.
func LocalOnlyBecause(t testing.TB, bug string) {
	t.Helper()
	if !bugPattern.MatchString(bug) {
		t.Fatal(`Local only tests are technical debt. You must include a valid bug number.`)
	}
	if IsRunningInCI() {
		t.Skipf("skipping test in CI due to %s", bug)
	} else {
		t.Logf("test is marked local-only due to %s", bug)
	}
}

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
	"testing"
)

func TestSkipCI(t *testing.T) {
	// t.Parallel -- can't be parallel, environment variable manipulation
	checkRan := false
	defer func() {
		if !checkRan {
			log.Fatal("TestDontSkipCI was ineffective")
		}
	}()
	didSkip := true

	t.Setenv("CI", "1")

	t.Setenv("CHROME_HEADLESS", "")
	t.Setenv("SWARMING_HEADLESS", "")

	t.Run("try to skip this test", func(t *testing.T) {
		Because(t, "b/1")
		didSkip = false
	})

	if !didSkip {
		t.Error("failed to skip :(")
	}
	checkRan = true
}

func TestDontSkipCI(t *testing.T) {
	// t.Parallel -- can't be parallel, environment variable manipulation
	checkRan := false
	defer func() {
		if !checkRan {
			log.Fatal("TestDontSkipCI was ineffective")
		}
	}()
	didSkip := true

	t.Setenv("CI", "")

	t.Setenv("CHROME_HEADLESS", "")
	t.Setenv("SWARMING_HEADLESS", "")

	t.Run("try to skip this test", func(t *testing.T) {
		Because(t, "b/1")
		didSkip = false
	})

	if didSkip {
		t.Error("should not have been skipped")
	}
	checkRan = true
}

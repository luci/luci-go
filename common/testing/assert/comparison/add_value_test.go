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

package comparison

import (
	"testing"

	"go.chromium.org/luci/common/testing/typed"
)

// TestAddValue tests adding a value to the values of a failure.
func TestAddValuef(t *testing.T) {
	t.Parallel()

	failure := &Failure{}
	AddFormattedFinding(failure, "a", "%d", 4)
	if diff := typed.Got(failure.Findings[0]).Want(&Failure_Finding{Name: "a", Value: []string{"4"}}).Diff(); diff != "" {
		t.Errorf("unexpected diff (-want +got): %s", diff)
	}
}

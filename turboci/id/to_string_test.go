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

package id

import (
	"fmt"

	"go.chromium.org/luci/common/clock/testclock"
)

func ExampleToString() {
	fmt.Println(ToString(SetWorkplan(Check("my-check"), "my-workplan")))
	fmt.Println(ToString(SetWorkplan(
		must(CheckEditOptionErr("some check", testclock.TestRecentTimeUTC, 10)),
		"the workplan")))
	// Output:
	// Lmy-workplan:Cmy-check
	// Lthe workplan:Csome check:V1454472306/7:O10
}

func ExampleToString_stages() {
	fmt.Println(ToString(SetWorkplan(Stage("my-stage"), "my-workplan")))
	fmt.Println(ToString(SetWorkplan(StageUnknown("my-stage"), "my-workplan")))
	fmt.Println(ToString(SetWorkplan(StageWorkNode("my-stage"), "my-workplan")))
	// Output:
	// Lmy-workplan:Smy-stage
	// Lmy-workplan:?my-stage
	// Lmy-workplan:Nmy-stage
}

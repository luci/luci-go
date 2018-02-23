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

package caching

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHashParams(t *testing.T) {
	t.Parallel()

	Convey(`Testing HashParams`, t, func() {
		// Can't cheat w/ strings including "\x00" (escaping).
		//
		// Without escaping:
		// ["A\x00B"] => A 0x00 B 0x00
		// ["A" "B"]  => A 0x00 B 0x00
		So(HashParams("A\x00B"), ShouldNotResemble, HashParams("A", "B"))

		// Can't cheat with different lengths (length prefix).
		//
		// After escaping, but with no length prefix:
		// ["A\x00" ""]     => A 0x00 0x00 0x00 0x00
		// ["A" "" "" ""]   => A 0x00 0x00 0x00 0x00
		So(HashParams("A\x00", ""), ShouldNotResemble, HashParams("A", "", "", ""))
	})
}

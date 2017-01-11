// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

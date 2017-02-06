// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package isolated

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHexDigestValid(t *testing.T) {
	t.Parallel()
	Convey(`Tests valid hex digest values.`, t, func() {
		valid := []string{
			"0123456789012345678901234567890123456789",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		for _, in := range valid {
			So(HexDigest(in).Validate(), ShouldBeTrue)
		}
	})
}

func TestHexDigestInvalid(t *testing.T) {
	t.Parallel()
	Convey(`Tests invalid hex digest values.`, t, func() {
		invalid := []string{
			"0123456789",
			"AAAAAAAAAA",
			"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
			"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		}
		for _, in := range invalid {
			So(HexDigest(in).Validate(), ShouldBeFalse)
		}
	})
}

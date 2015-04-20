// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolated

import (
	"testing"

	"github.com/maruel/ut"
)

func TestHexDigestValid(t *testing.T) {
	t.Parallel()
	valid := []string{
		"0123456789012345678901234567890123456789",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	for i, in := range valid {
		ut.AssertEqualIndex(t, i, true, HexDigest(in).Validate())
	}
}

func TestHexDigestInvalid(t *testing.T) {
	t.Parallel()
	invalid := []string{
		"0123456789",
		"AAAAAAAAAA",
		"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	for i, in := range invalid {
		ut.AssertEqualIndex(t, i, false, HexDigest(in).Validate())
	}
}

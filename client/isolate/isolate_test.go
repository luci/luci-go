// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"testing"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/maruel/ut"
)

func TestReplaceVars(t *testing.T) {
	t.Parallel()

	opts := ArchiveOptions{PathVariables: common.KeyValVars{"VAR": "wonderful"}}

	// Single replacement.
	r, err := ReplaceVariables("hello <(VAR) world", opts)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "hello wonderful world", r)

	// Multiple replacement.
	r, err = ReplaceVariables("hello <(VAR) <(VAR) world", opts)
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, "hello wonderful wonderful world", r)

	// Replacement of missing variable.
	r, err = ReplaceVariables("hello <(MISSING) world", opts)
	ut.AssertEqual(t, "no value for variable 'MISSING'", err.Error())
}

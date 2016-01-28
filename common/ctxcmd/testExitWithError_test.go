// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ctxcmd

import (
	"os"
	"testing"
)

func TestExitWithError(t *testing.T) {
	if !isHelperTest() {
		return
	}

	os.Exit(42)
}

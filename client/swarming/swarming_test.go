// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"testing"

	"github.com/maruel/ut"
)

func TestNew(t *testing.T) {
	t.Parallel()
	// TODO(maruel): Make a fake.
	_, err := New("https://localhost:1")
	ut.AssertEqual(t, nil, err)
}

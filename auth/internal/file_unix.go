// Copyright 2017 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !windows

package internal

import (
	"os"
)

func openSharedDelete(name string) (*os.File, error) {
	return os.Open(name)
}

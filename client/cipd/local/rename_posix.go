// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !windows

package local

import (
	"os"
)

func atomicRename(source, target string) error {
	return os.Rename(source, target)
}

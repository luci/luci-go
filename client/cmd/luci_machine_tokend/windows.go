// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build windows

package main

import (
	"os"
)

func interruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !appengine

package logging

import "github.com/Sirupsen/logrus"

func ExampleLogrusLogger() {
	// If it compiles, then logrus.Logger interface matches Logger interface.
	var _ Logger = logrus.New()
}

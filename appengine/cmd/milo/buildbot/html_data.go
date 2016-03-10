// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

import (
	"github.com/luci/luci-go/appengine/cmd/milo/settings"
)

// TestableBuild is a subclass of Build that interfaces with TestableHandler and
// includes sample test data.
type TestableBuild Build

// TestableBuilder is a subclass of Builder that interfaces with TestableHandler
// and includes sample test data.
type TestableBuilder Builder

// TestData returns sample test data.
func (b Build) TestData() []settings.TestBundle {
	return []settings.TestBundle{}
}

// TestData returns sample test data.
func (b Builder) TestData() []settings.TestBundle {
	return []settings.TestBundle{}
}

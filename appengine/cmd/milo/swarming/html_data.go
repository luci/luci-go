// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/appengine/cmd/milo/settings"
	"github.com/luci/luci-go/server/templates"
)

// TestableLog is a subclass of Log that interfaces with TestableHandler and
// includes sample test data.
type TestableLog struct{ Log }

// TestableBuild is a subclass of Build that interfaces with TestableHandler and
// includes sample test data.
type TestableBuild struct{ Build }

// TestData returns sample test data.
func (l TestableLog) TestData() []settings.TestBundle {
	return []settings.TestBundle{}
}

// TestData returns sample test data.
func (b TestableBuild) TestData() []settings.TestBundle {
	basic := resp.MiloBuild{
		Summary: resp.BuildComponent{
			Label:  "Test swarming build",
			Status: resp.Success,
		},
	}
	return []settings.TestBundle{
		{
			Description: "Basic successful build",
			Data:        templates.Args{"Build": basic},
		},
	}
}

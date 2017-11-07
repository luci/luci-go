// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildsource

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/milo/frontend/ui"
)

// ID represents a universal 'build' ID. Each subpackage of buildsource
// implements an ID (as the type BuildID, e.g. swarming.BuildID), which has
// buildsource-specific fields, but always implements this ID interface.
//
// The frontend constructs an ID from the appropriate buildsource, then calls
// its .Get() method to get the generic MiloBuild representation.
type ID interface {
	Get(c context.Context) (*ui.MiloBuild, error)

	// GetLog is only implemented by swarming; this is for serving the deprecated
	//   swarming/task/<id>/steps/<logname>
	// API. Once that's removed, this should be removed as well.
	GetLog(c context.Context, logname string) (text string, closed bool, err error)
}

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !appengine

package prod

import (
	"golang.org/x/net/context"
	"google.golang.org/appengine"
)

// UseBackground is the same as Use except that it activates production
// implementations which aren't associated with any particular request.
//
// This is only available on Managed VMs.
func UseBackground(c context.Context) context.Context {
	return setupAECtx(c, appengine.BackgroundContext())
}

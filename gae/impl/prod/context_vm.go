// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
//
// It is important to note that this DOES NOT install the AppEngine SDK into the
// supplied Context. See the warning in Use for more information.
func UseBackground(c context.Context) context.Context {
	return setupAECtx(c, appengine.BackgroundContext())
}

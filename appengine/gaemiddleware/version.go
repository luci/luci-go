// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gaemiddleware

import (
	"github.com/luci/luci-go/common/tsmon/versions"
)

// Version is a semantic version of base luci-go GAE library.
//
// It is bumped whenever we add new features or fix important bugs. It is
// reported to monitoring as 'luci/components/version' string metric with
// 'component' field set to 'github.com/luci/luci-go/appengine/gaemiddleware'.
//
// It allows to track what GAE apps use what version of the library, so it's
// easier to detect stale code running in production.
const Version = "1.0.0"

func init() {
	versions.Register("github.com/luci/luci-go/appengine/gaemiddleware", Version)
}

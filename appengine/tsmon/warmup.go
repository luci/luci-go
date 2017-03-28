// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tsmon

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/warmup"
)

func init() {
	warmup.Register("appengine/tsmon", func(c context.Context) (err error) {
		if settings := fetchCachedSettings(c); settings.Enabled && settings.ProdXAccount != "" {
			err = canActAsProdX(c, settings.ProdXAccount)
		} else {
			logging.Infof(c, "Skipping tsmon warmup, not configured")
		}
		return
	})
}

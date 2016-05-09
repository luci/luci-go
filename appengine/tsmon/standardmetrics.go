// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"golang.org/x/net/context"

	"github.com/luci/gae/service/module"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/metric"
)

var (
	defaultVersion = metric.NewCallbackString(
		"appengine/default_version",
		"Name of the version currently marked as default.")
)

func standardMetricsCallback(c context.Context) {
	version, err := module.Get(c).DefaultVersion("")
	if err != nil {
		logging.Errorf(c, "Error getting default appengine version: %s", err)
		defaultVersion.Set(c, "(unknown)")
	} else {
		defaultVersion.Set(c, version)
	}
}

func init() {
	tsmon.RegisterGlobalCallback(standardMetricsCallback, defaultVersion)
}

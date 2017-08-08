// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tsmon

import (
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/module"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/runtimestats"
	"go.chromium.org/luci/common/tsmon/versions"
)

var (
	defaultVersion = metric.NewCallbackString(
		"appengine/default_version",
		"Name of the version currently marked as default.",
		nil)
)

// collectGlobalMetrics populates service-global metrics.
//
// Called by tsmon from inside /housekeeping cron handler. Metrics reported must
// not depend on the state of the particular process that happens to report
// them.
func collectGlobalMetrics(c context.Context) {
	version, err := module.DefaultVersion(c, "")
	if err != nil {
		logging.Errorf(c, "Error getting default appengine version: %s", err)
		defaultVersion.Set(c, "(unknown)")
	} else {
		defaultVersion.Set(c, version)
	}
}

// collectProcessMetrics populates per-process metrics.
//
// It is called by each individual process right before flushing the metrics.
func collectProcessMetrics(c context.Context, s *tsmonSettings) {
	versions.Report(c)
	if s.ReportRuntimeStats {
		runtimestats.Report(c)
	}
}

func init() {
	tsmon.RegisterGlobalCallback(collectGlobalMetrics, defaultVersion)
}

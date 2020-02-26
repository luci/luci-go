// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package jobcreate

import (
	"time"

	"github.com/golang/protobuf/ptypes"
	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/led/job"
)

func bbCommonFromTaskRequest(bb *job.Buildbucket, r *swarming.SwarmingRpcsNewTaskRequest) {
	ts := r.TaskSlices[0]

	bb.EnsureBasics()

	bb.CipdPackages = cipdPins(ts.Properties.CipdInput)
	bb.EnvVars = strPairs(ts.Properties.Env)
	bb.EnvPrefixes = strListPairs(ts.Properties.EnvPrefixes)

	bb.GracePeriod = ptypes.DurationProto(
		time.Second * time.Duration(ts.Properties.GracePeriodSecs))

	bb.BbagentArgs.Build.SchedulingTimeout = ptypes.DurationProto(
		time.Second * time.Duration(r.ExpirationSecs))
	bb.BotPingTolerance = ptypes.DurationProto(
		time.Second * time.Duration(r.BotPingToleranceSecs))

	bb.Containment = containmentFromSwarming(ts.Properties.Containment)
}

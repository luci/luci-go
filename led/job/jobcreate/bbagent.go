// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package jobcreate

import (
	"time"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/led/job"
)

func jobDefinitionFromBuildbucket(bb *job.Buildbucket, r *swarming.SwarmingRpcsNewTaskRequest) (err error) {
	bbCommonFromTaskRequest(bb, r)
	bb.BbagentArgs, err = bbinput.Parse(r.TaskSlices[0].Properties.Command[1])
	return
}

func bbCommonFromTaskRequest(bb *job.Buildbucket, r *swarming.SwarmingRpcsNewTaskRequest) {
	ts := r.TaskSlices[0]

	bb.EnsureBasics()

	bb.CipdPackages = cipdPins(ts.Properties.CipdInput)
	bb.EnvVars = strPairs(ts.Properties.Env)
	bb.EnvPrefixes = strListPairs(ts.Properties.EnvPrefixes)

	bb.GracePeriod = ptypes.DurationProto(
		time.Second * time.Duration(ts.Properties.GracePeriodSecs))

	bb.TotalExpiration = ptypes.DurationProto(
		time.Second * time.Duration(r.ExpirationSecs))
	bb.BotPingTolerance = ptypes.DurationProto(
		time.Second * time.Duration(r.BotPingToleranceSecs))

	bb.Containment = containmentFromSwarming(ts.Properties.Containment)
}

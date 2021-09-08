// Copyright 2020 The LUCI Authors.
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

package handler

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/usertext"
)

// Start implements Handler interface.
func (*Impl) Start(ctx context.Context, rs *state.RunState) (*Result, error) {
	switch status := rs.Run.Status; {
	case status == run.Status_STATUS_UNSPECIFIED:
		err := errors.Reason("CRITICAL: can't start a Run %q with unspecified status", rs.Run.ID).Err()
		common.LogError(ctx, err)
		panic(err)
	case status != run.Status_PENDING:
		logging.Debugf(ctx, "Skip starting Run because this Run is %s", status)
		return &Result{State: rs}, nil
	}

	cg, err := rs.LoadConfigGroup(ctx)
	if err != nil {
		return nil, err
	}

	res := &Result{State: rs.ShallowCopy()}
	res.State.Run.Status = run.Status_RUNNING
	res.State.Run.StartTime = clock.Now(ctx).UTC()

	res.State.LogEntries = append(res.State.LogEntries, &run.LogEntry{
		Time: timestamppb.New(res.State.Run.StartTime),
		Kind: &run.LogEntry_Started_{
			Started: &run.LogEntry_Started{},
		},
	})

	recordPickupLatency(ctx, &(res.State.Run), cg)
	return res, nil
}

func recordPickupLatency(ctx context.Context, r *run.Run, cg *prjcfg.ConfigGroup) {
	delay := r.StartTime.Sub(r.CreateTime)
	metricPickupLatencyS.Add(ctx, delay.Seconds(), r.ID.LUCIProject())

	if d := cg.Content.GetCombineCls().GetStabilizationDelay(); d != nil {
		delay -= d.AsDuration()
	}
	metricPickupLatencyAdjustedS.Add(ctx, delay.Seconds(), r.ID.LUCIProject())

	if delay >= 1*time.Minute {
		logging.Warningf(ctx, "Too large adjusted pickup delay: %s", delay)
	}
}

type startMessageMaker func(clid common.CLID) (string, error)

func startMessageFactory(r *run.Run, cls []*run.RunCL) startMessageMaker {
	clidToIndex := make(map[common.CLID]int, len(cls))
	botdataCLs := make([]botdata.ChangeID, len(cls))
	for i, cl := range cls {
		if r.CLs[i] != cl.ID {
			panic(fmt.Errorf("run entity CLs don't correspond to RunCLs entities at index %d: %d vs %d", i, r.CLs[i], cl.ID))
		}
		clidToIndex[cl.ID] = i
		g := cl.Detail.GetGerrit()
		if g == nil {
			panic(fmt.Errorf("CL %d isn't a Gerrit CL and/or corrutped RunCL", cl.ID))
		}
		botdataCLs[i] = botdata.ChangeID{
			Host:   g.GetHost(),
			Number: g.GetInfo().GetNumber(),
		}
	}
	humanMsg := usertext.OnRunStarted(r.Mode)
	return func(clid common.CLID) (string, error) {
		index, ok := clidToIndex[clid]
		if !ok {
			panic(fmt.Errorf("CL %d doesn't belong to Run %q", clid, r.ID))
		}
		g := cls[index].Detail.GetGerrit()
		bd := botdata.BotData{
			Action:      botdata.Start,
			CLs:         botdataCLs,
			Revision:    g.GetInfo().GetCurrentRevision(),
			TriggeredAt: cls[index].Trigger.GetTime().AsTime(),
		}
		return botdata.Append(humanMsg, bd)
	}
}

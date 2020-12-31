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

package build

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// StepView is a window into the build State.
//
// You can obtain/manipulate this with the Step.Modify method.
type StepView struct {
	SummaryMarkdown string
}

// Start will change the status of this Step from SCHEDULED to STARTED and
// initializes StartTime.
//
// This must only be called for ScheduleStep invocations. If the step is already
// started (e.g. it was produced via StartStep() or Start() was already called),
// this panics.
func (s *Step) Start() {
	s.mutate(func() {
		if status := s.stepPb.GetStatus(); status != bbpb.Status_SCHEDULED {
			panic(errors.Reason("cannot start step %q: not SCHEDULED: %s", s.stepPb.Name, status).Err())
		}

		s.stepPb.Status = bbpb.Status_STARTED
		s.stepPb.StartTime = timestamppb.New(clock.Now(s.ctx))

		if s.noopMode() /* || no logsink */ {
			logging.Infof(s.ctx, "set status: %s", bbpb.Status_STARTED)
		}
	})
}

// Modify allows you to atomically manipulate the StepView for this Step.
//
// Blocking in Modify will block other callers of Modify and Set*, as well as
// the ability for the build State to be sent (with the function set by
// OptSend).
//
// The Set* methods should be preferred unless you need to read/modify/write
// View items.
func (s *Step) Modify(cb func(*StepView)) {
	logSM := ""
	s.mutate(func() {
		oldView := StepView{s.stepPb.SummaryMarkdown}
		newView := oldView // shallow copy
		cb(&newView)
		if oldView.SummaryMarkdown != newView.SummaryMarkdown {
			logSM = newView.SummaryMarkdown
			s.stepPb.SummaryMarkdown = newView.SummaryMarkdown
		}
	})
	if s.noopMode() && len(logSM) > 0 {
		logging.Debugf(s.ctx, "changed SummaryMarkdown: %s", logSM)
	}
}

// SetSummaryMarkdown atomically sets the step's SummaryMarkdown field.
func (s *Step) SetSummaryMarkdown(summaryMarkdown string) {
	s.Modify(func(v *StepView) {
		v.SummaryMarkdown = summaryMarkdown
	})
}

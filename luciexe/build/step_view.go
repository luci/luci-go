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
	"sort"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/logging"
)

// StepView is a window into the build State.
//
// You can obtain/manipulate this with the Step.Modify method.
type StepView struct {
	SummaryMarkdown string
	Tags            map[string][]string
}

// Start will change the status of this Step from SCHEDULED to STARTED and
// initializes StartTime.
//
// This must only be called for ScheduleStep invocations. If the step is already
// started (e.g. it was produced via StartStep() or Start() was already called),
// this does nothing.
func (s *Step) Start() {
	s.mutate(nil)
}

// Modify allows you to atomically manipulate the StepView for this Step.
//
// Blocking in Modify will block other callers of Modify and Set*, as well as
// the ability for the build State to be sent (with the function set by
// OptSend).
//
// The Set* methods should be preferred unless you need to read/modify/write
// View items.
//
// This starts the step if it's still SCHEDULED.
func (s *Step) Modify(cb func(*StepView)) {
	logSM := ""
	logTags := []*pb.StringPair{}
	s.mutate(func() bool {
		// Creating a deep copy of tags
		// note: we keep pb.Tags sorted at all times
		tags := convertPbTagsToMap(s.stepPb.Tags)
		view := StepView{s.stepPb.SummaryMarkdown, tags}
		cb(&view)

		// callback w/ tags
		tagOut := convertMapTagsToPb(view.Tags)

		isTagsMod := cmpPbTags(s.stepPb.Tags, view.Tags)
		modified := false

		if len(s.stepPb.SummaryMarkdown) != len(view.SummaryMarkdown) {
			logSM = view.SummaryMarkdown
			s.stepPb.SummaryMarkdown = view.SummaryMarkdown
			modified = true
		}
		if isTagsMod {
			// TODO: @randymaldonado change this to a diff between
			// tagOut and s.stepPb.Tags.
			logTags = tagOut
			s.stepPb.Tags = tagOut
			modified = true
		}
		return modified
	})
	if s.logsink() == nil {
		if len(logSM) > 0 {
			logging.Debugf(s.ctx, "changed SummaryMarkdown: %s", logSM)
		}
		if len(logTags) > 0 {
			logging.Debugf(s.ctx, "changed Tags: %s", logTags)
		}
	}
}

func convertMapTagsToPb(tags map[string][]string) []*pb.StringPair {
	tagOut := make([]*pb.StringPair, 0, len(tags))
	for key, vals := range tags {
		for _, val := range vals {
			tagOut = append(tagOut, &pb.StringPair{Key: key, Value: val})
		}
	}
	sort.Slice(tagOut, sortby.Chain{
		func(i, j int) bool { return tagOut[i].Key < tagOut[j].Key },
		func(i, j int) bool { return tagOut[i].Value < tagOut[j].Value },
	}.Use)
	return tagOut
}

func convertPbTagsToMap(tags []*pb.StringPair) map[string][]string {
	tagsMap := make(map[string][]string, len(tags))
	for _, pair := range tags {
		tagsMap[pair.Key] = append(tagsMap[pair.Key], pair.Value)
	}
	return tagsMap
}

func cmpPbTags(oldTagsPb []*pb.StringPair, newTagsMap map[string][]string) bool {
	tags := convertPbTagsToMap(oldTagsPb)
	if len(tags) != len(newTagsMap) {
		return true
	}
	for key, vals := range newTagsMap {
		if len(vals) != len(tags[key]) {
			return true
		}
	}
	return false
}

// SetSummaryMarkdown atomically sets the step's SummaryMarkdown field.
func (s *Step) SetSummaryMarkdown(summaryMarkdown string) {
	s.Modify(func(v *StepView) {
		v.SummaryMarkdown = summaryMarkdown
	})
}

// AddTagValue sets the step's tag field by appending the new tag to the existing list.
func (s *Step) AddTagValue(key, value string) {
	if key != "" && value != "" {
		s.Modify(func(v *StepView) {
			vals := v.Tags[key]
			for _, val := range vals {
				if val == value {
					return // already there
				}
			}
			v.Tags[key] = append(vals, value)
		})
	}
}

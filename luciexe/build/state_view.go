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
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"google.golang.org/protobuf/proto"
)

// View is a window into the build State.
//
// You can obtain/manipulate this with the State.Modify method.
type View struct {
	SummaryMarkdown string
	Critical        bbpb.Trinary
	GitilesCommit   *bbpb.GitilesCommit
}

// Modify allows you to atomically manipulate the View on the build State.
//
// Blocking in Modify will block other callers of Modify and Set*, as well as
// the ability for the build State to be sent (with the function set by
// OptSend).
//
// The Set* methods should be preferred unless you need to read/modify/write
// View items.
func (s *State) Modify(cb func(*View)) {
	s.mutate(func() bool {
		v := View{
			s.buildPb.SummaryMarkdown,
			s.buildPb.Critical,
			nil,
		}
		if s.buildPb.Output.GitilesCommit != nil {
			v.GitilesCommit = proto.Clone(s.buildPb.Output.GitilesCommit).(*bbpb.GitilesCommit)
		}

		cb(&v)
		modified := false
		if v.SummaryMarkdown != s.buildPb.SummaryMarkdown {
			modified = true
			//summary := v.SummaryMarkdown
			if s.buildPb.Output == nil {
				s.buildPb.Output = &bbpb.Build_Output{}
			}
			s.buildPb.Output.SummaryMarkdown = v.SummaryMarkdown
			s.buildPb.SummaryMarkdown = v.SummaryMarkdown
		}
		if v.Critical != s.buildPb.Critical {
			modified = true
			s.buildPb.Critical = v.Critical
		}
		if !proto.Equal(v.GitilesCommit, s.buildPb.Output.GitilesCommit) {
			modified = true
			s.buildPb.Output.GitilesCommit = proto.Clone(v.GitilesCommit).(*bbpb.GitilesCommit)
		}
		return modified
	})
}

// SetSummaryMarkdown atomically sets the build's SummaryMarkdown field.
func (s *State) SetSummaryMarkdown(summaryMarkdown string) {
	s.Modify(func(v *View) {
		v.SummaryMarkdown = summaryMarkdown
	})
}

// SetCritical atomically sets the build's Critical field.
func (s *State) SetCritical(critical bbpb.Trinary) {
	s.Modify(func(v *View) {
		v.Critical = critical
	})
}

// SetGitilesCommit atomically sets the GitilesCommit.
//
// This will make a copy of the GitilesCommit object to store in the build
// State.
func (s *State) SetGitilesCommit(gc *bbpb.GitilesCommit) {
	s.Modify(func(v *View) {
		v.GitilesCommit = gc
	})
}

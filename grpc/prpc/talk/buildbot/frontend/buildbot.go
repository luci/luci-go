// Copyright 2015 The LUCI Authors.
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

package main

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	buildbot "go.chromium.org/luci/grpc/prpc/talk/buildbot/proto"
)

type buildbotService struct{}

func (s *buildbotService) Search(c context.Context, req *buildbot.SearchRequest) (*buildbot.SearchResponse, error) {
	allBuilds := []*buildbot.Build{
		{
			Master:  "master.tryserver.chromium.linux",
			Builder: "Debug",
			Number:  1,
			State:   buildbot.BuildState_PENDING,
		},
		{
			Master:  "master.tryserver.chromium.linux",
			Builder: "Release",
			Number:  1,
			State:   buildbot.BuildState_RUNNING,
		},
		{
			Master:  "master.tryserver.chromium.win",
			Builder: "Debug",
			Number:  1,
			State:   buildbot.BuildState_SUCCESS,
		},
	}
	res := &buildbot.SearchResponse{}
	for _, b := range allBuilds {
		if req.Master != "" && b.Master != "" {
			continue
		}
		if req.State != buildbot.BuildState_UNSET && b.State != req.State {
			continue
		}
		if req.Builder != "" && b.Builder != req.Builder {
			continue
		}
		res.Builds = append(res.Builds, b)
	}
	return res, nil
}

func (s *buildbotService) Schedule(c context.Context, req *buildbot.ScheduleRequest) (*buildbot.ScheduleResponse, error) {
	if req.Master == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Master not specified")
	}

	res := &buildbot.ScheduleResponse{}
	for i, b := range req.GetBuilds() {
		if b.Builder == "" {
			return nil, status.Errorf(codes.InvalidArgument, "Builder not specified")
		}

		res.Builds = append(res.Builds, &buildbot.Build{
			Master:  req.Master,
			Builder: b.Builder,
			Number:  int32(i) + 1,
			State:   buildbot.BuildState_PENDING,
		})
	}
	return res, nil
}

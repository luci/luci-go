// Copyright 2023 The LUCI Authors.
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

package rpc

import (
	"context"

	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

type buganizerTesterServer struct{}

func NewBuganizerTesterServer() *pb.DecoratedBuganizerTester {
	return &pb.DecoratedBuganizerTester{
		Prelude:  checkAllowedAdminPrelude,
		Service:  &buganizerTesterServer{},
		Postlude: GRPCifyAndLogPostlude,
	}
}

func (b buganizerTesterServer) CreateSampleIssue(ctx context.Context, req *pb.CreateSampleIssueRequest) (rsp *pb.CreateSampleIssueResponse, err error) {
	issueId, err := buganizer.CreateSampleIssue(ctx)

	if err != nil {
		return nil, err
	}

	return &pb.CreateSampleIssueResponse{
		IssueId: issueId,
	}, nil
}

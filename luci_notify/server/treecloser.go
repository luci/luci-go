// Copyright 2022 The LUCI Authors.
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

// Package server implements the server to handle pRPC requests.
package server

import (
	"context"
	"fmt"
	"regexp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/config"
)

// TreeCloserServer implements the proto service TreeClosers.
type TreeCloserServer struct{}

// CheckTreeCloser Checks if the builder in CheckTreeCloserRequest is tree-closer or not
func (server *TreeCloserServer) CheckTreeCloser(c context.Context, req *pb.CheckTreeCloserRequest) (*pb.CheckTreeCloserResponse, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	// Query tree closers based on project, bucket
	ancestorKey := datastore.MakeKey(c, "Project", req.Project, "Builder", fmt.Sprintf("%s/%s", req.Bucket, req.Builder))
	q := datastore.NewQuery("TreeCloser").Ancestor(ancestorKey)
	treeClosers := []*config.TreeCloser{}
	err := datastore.GetAll(c, q, &treeClosers)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "couldn't query tree closers: %v", err)
	}

	if len(treeClosers) == 0 {
		return &pb.CheckTreeCloserResponse{
			IsTreeCloser: false,
		}, nil
	}

	for _, treeCloser := range treeClosers {
		if stepMatchesRule(req.Step, treeCloser.TreeCloser.FailedStepRegexp, treeCloser.TreeCloser.FailedStepRegexpExclude) {
			return &pb.CheckTreeCloserResponse{
				IsTreeCloser: true,
			}, nil
		}
	}
	return &pb.CheckTreeCloserResponse{
		IsTreeCloser: false,
	}, nil
}

func stepMatchesRule(stepName string, failedStepRegexp string, failedStepRegexpExclude string) bool {
	var includeRegex *regexp.Regexp
	if failedStepRegexp != "" {
		// We should never get an invalid regex here, as our validation should catch this.
		includeRegex = regexp.MustCompile(fmt.Sprintf("^%s$", failedStepRegexp))
	}

	var excludeRegex *regexp.Regexp
	if failedStepRegexpExclude != "" {
		excludeRegex = regexp.MustCompile(fmt.Sprintf("^%s$", failedStepRegexpExclude))
	}

	return (includeRegex == nil || includeRegex.MatchString(stepName)) && (excludeRegex == nil || !excludeRegex.MatchString(stepName))
}

func validateRequest(req *pb.CheckTreeCloserRequest) error {
	if req.Project == "" {
		return status.Errorf(codes.InvalidArgument, "project cannot be empty")
	}
	if req.Bucket == "" {
		return status.Errorf(codes.InvalidArgument, "bucket cannot be empty")
	}
	if req.Builder == "" {
		return status.Errorf(codes.InvalidArgument, "builder cannot be empty")
	}
	if req.Step == "" {
		return status.Errorf(codes.InvalidArgument, "step cannot be empty")
	}
	return nil
}

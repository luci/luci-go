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
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

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

// NotifyCulpritRevert handles notification from luci-bisection that an automated
// revert has landed. This creates a CulpritRevertEvent entity that will be
// processed by the tree status update cron job.
func (server *TreeCloserServer) NotifyCulpritRevert(c context.Context, req *pb.NotifyCulpritRevertRequest) (*emptypb.Empty, error) {
	if err := validateNotifyCulpritRevertRequest(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	logging.Infof(c, "Received culprit revert notification for tree %s: culprit=%s, revert=%s",
		req.TreeName, req.CulpritReviewUrl, req.RevertReviewUrl)

	newRevertTime := req.RevertLandTime.AsTime()

	// Check if there's an existing event for this tree
	existingEvent := &config.CulpritRevertEvent{TreeName: req.TreeName}
	err := datastore.Get(c, existingEvent)
	if err != nil && err != datastore.ErrNoSuchEntity {
		logging.Errorf(c, "Failed to check existing CulpritRevertEvent: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to check existing culprit revert event: %v", err)
	}

	// Only store if this revert is more recent than any existing revert
	if err == nil && !newRevertTime.After(existingEvent.RevertLandTime) {
		logging.Infof(c, "Ignoring revert notification for tree %s: new revert time %v is not after existing revert time %v",
			req.TreeName, newRevertTime, existingEvent.RevertLandTime)
		return &emptypb.Empty{}, nil
	}

	event := &config.CulpritRevertEvent{
		TreeName:         req.TreeName,
		RevertLandTime:   newRevertTime.UTC(),
		CulpritReviewURL: req.CulpritReviewUrl,
		RevertReviewURL:  req.RevertReviewUrl,
		CreatedAt:        time.Now().UTC(),
	}

	if err := datastore.Put(c, event); err != nil {
		logging.Errorf(c, "Failed to save CulpritRevertEvent: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to save culprit revert event: %v", err)
	}

	logging.Infof(c, "Successfully saved CulpritRevertEvent for tree %s (revert time: %v)", req.TreeName, newRevertTime)
	return &emptypb.Empty{}, nil
}

const (
	// maxURLLength is the maximum length for URL fields (RFC 2616 suggests 2083 bytes)
	maxURLLength = 2083
	// maxClockDrift is the maximum allowed drift for RevertLandTime into the future
	maxClockDrift = 10 * time.Second
)

func validateNotifyCulpritRevertRequest(req *pb.NotifyCulpritRevertRequest) error {
	if req.TreeName == "" {
		return errors.New("tree_name: unspecified")
	}
	if !config.TreeNameRE.MatchString(req.TreeName) {
		return errors.Fmt("tree_name: must match pattern %s", config.TreeNameRE.String())
	}

	if req.RevertLandTime == nil {
		return errors.New("revert_land_time: unspecified")
	}
	revertTime := req.RevertLandTime.AsTime()
	if revertTime.After(time.Now().Add(maxClockDrift)) {
		return errors.Fmt("revert_land_time: must not be more than %v in the future", maxClockDrift)
	}

	if req.CulpritReviewUrl == "" {
		return errors.New("culprit_review_url: unspecified")
	}
	if len(req.CulpritReviewUrl) > maxURLLength {
		return errors.Fmt("culprit_review_url: exceeds maximum length of %d bytes", maxURLLength)
	}

	if req.RevertReviewUrl == "" {
		return errors.New("revert_review_url: unspecified")
	}
	if len(req.RevertReviewUrl) > maxURLLength {
		return errors.Fmt("revert_review_url: exceeds maximum length of %d bytes", maxURLLength)
	}

	return nil
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

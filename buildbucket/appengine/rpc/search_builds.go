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

package rpc

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/internal/search"
	"go.chromium.org/luci/buildbucket/appengine/model"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// validateChange validates the given Gerrit change.
func validateChange(ch *pb.GerritChange) error {
	switch {
	case ch.GetHost() == "":
		return errors.Reason("host is required").Err()
	case ch.GetChange() == 0:
		return errors.Reason("change is required").Err()
	case ch.GetPatchset() == 0:
		return errors.Reason("patchset is required").Err()
	default:
		return nil
	}
}

// validatePredicate validates the given build predicate.
func validatePredicate(pr *pb.BuildPredicate) error {
	if b := pr.GetBuilder(); b != nil {
		switch err := protoutil.ValidateBuilderID(b); {
		case err != nil:
			return errors.Annotate(err, "builder").Err()
		case b.Bucket == "" && b.Builder != "":
			return errors.Annotate(errors.Reason("bucket is required").Err(), "builder").Err()
		}
	}
	for i, ch := range pr.GetGerritChanges() {
		if err := validateChange(ch); err != nil {
			return errors.Annotate(err, "gerrit_change[%d]", i).Err()
		}
	}
	if c := pr.GetOutputGitilesCommit(); c != nil {
		if err := validateCommit(c); err != nil {
			return errors.Annotate(err, "output_gitiles_commit").Err()
		}
	}
	if pr.GetBuild() != nil && pr.CreateTime != nil {
		return errors.Reason("build is mutually exclusive with create_time").Err()
	}
	// TODO(crbug/1053813): Disallow empty predicate.
	return nil
}

// validatePageToken validates the given page token.
func validatePageToken(token string) error {
	if token != "" && !search.PageTokenRegex.MatchString(token) {
		return errors.Reason("invalid page_token").Err()
	}
	return nil
}

// validateSearch validates the given request.
func validateSearch(req *pb.SearchBuildsRequest) error {
	if err := validatePageSize(req.GetPageSize()); err != nil {
		return err
	}

	if err := validatePageToken(req.GetPageToken()); err != nil {
		return err
	}

	if err := validatePredicate(req.GetPredicate()); err != nil {
		return errors.Annotate(err, "predicate").Err()
	}
	return nil
}

// SearchBuilds handles a request to search for builds. Implements pb.BuildsServer.
func (*Builds) SearchBuilds(ctx context.Context, req *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
	// TODO(crbug/1090540): clean up code after 100% migration in case it may be used later.
	// pyRes, pyErr := verifySearch(ctx, req)
	// if !bbClientErr.In(pyErr) {
	// 	return pyRes, pyErr
	// }
	// logging.Warningf(ctx, "search verification: errors in py client - %v\n Falling back to Go path.", pyErr)

	if err := validateSearch(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	mask, err := getBuildsSubMask(req.GetFields())
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "fields").Err())
	}

	rsp, err := search.NewQuery(req).Fetch(ctx)
	if err != nil {
		return nil, err
	}
	if err = model.LoadBuildDetails(ctx, mask, rsp.Builds...); err != nil {
		return nil, err
	}
	return rsp, nil
}

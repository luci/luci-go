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
	"strings"

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

// validateCommit validates the given Gitiles commit.
func validateCommit(cm *pb.GitilesCommit) error {
	switch {
	case cm.GetHost() == "":
		return errors.Reason("host is required").Err()
	case cm.GetProject() == "":
		return errors.Reason("project is required").Err()
	case cm.GetId() != "":
		switch {
		case cm.Ref != "" || cm.Position != 0:
			return errors.Reason("id is mutually exclusive with (ref and position)").Err()
		case !sha1Regex.MatchString(cm.Id):
			return errors.Reason("id must match %q", sha1Regex).Err()
		}
	case cm.GetRef() != "":
		if !strings.HasPrefix(cm.Ref, "refs/") {
			return errors.Reason("ref must match refs/.*").Err()
		}
	default:
		return errors.Reason("one of id or ref is required").Err()
	}
	return nil
}

// validatePredicate validates the given build predicate.
func validatePredicate(pr *pb.BuildPredicate) error {
	if b := pr.GetBuilder(); b != nil {
		if err := protoutil.ValidateBuilderID(b); err != nil {
			return errors.Annotate(err, "builder").Err()
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
	// TODO(crbug/1090540): validate the pr.createBy identity.
	// It'll be the replacement for the `user.parse_identity(q.created_by)` in Py.
	// https://source.chromium.org/chromium/infra/infra/+/master:appengine/cr-buildbucket/search.py;l=294
	return nil
}

// validatePageToken checks if there are indexed tags when the token is TagIndex token.
func validatePageToken(token string, tags []*pb.StringPair) error {
	if search.TagIndexCursorRegex.MatchString(token) && len(search.IndexedTags(protoutil.StringPairMap(tags))) == 0 {
		return errors.Reason("invalid page_token").Err()
	}
	return nil
}

// validateSearch validates the given request.
func validateSearch(req *pb.SearchBuildsRequest) error {
	if err := validatePageSize(req.GetPageSize()); err != nil {
		return err
	}

	if err := validatePageToken(req.GetPageToken(), req.GetPredicate().GetTags()); err != nil {
		return err
	}

	if err := validatePredicate(req.GetPredicate()); err != nil {
		return errors.Annotate(err, "predicate").Err()
	}
	return nil
}

// SearchBuilds handles a request to search for builds. Implements pb.BuildsServer.
func (*Builds) SearchBuilds(ctx context.Context, req *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
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
	if err = model.LoadBuildBundles(ctx, rsp.Builds, mask); err != nil {
		return nil, err
	}
	return rsp, nil
}

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
	"regexp"
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	imodel "go.chromium.org/luci/buildbucket/appengine/internal/model"
	dbmodel "go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var (
	tagIndexCursorRegex = regexp.MustCompile("^id>\\d+$")
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
	// TODO(crbug/1042991): Potentially validate tags (buildset is validated in Python SearchBuilds).
	// TODO(crbug/1053813): Disallow empty predicate.
	return nil
}

// validateSearch validates the given request.
func validateSearch(req *pb.SearchBuildsRequest) error {
	if err := validatePageSize(req.GetPageSize()); err != nil {
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
	_, err := getFieldMask(req.GetFields())
	if err != nil {
		return nil, appstatus.BadRequest(errors.Annotate(err, "fields").Err())
	}
	// TODO(crbug/1042991): Search for the requested builds.
	searchQuery, err:= imodel.NewSearchQuery(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	_, err = searchBuilds(ctx, searchQuery)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func searchBuilds(ctx context.Context, q *imodel.SearchQuery) (*pb.SearchBuildsResponse, error) {
	// TODO(crbug/1090540): validate the q.createBy identity when validating the RPC req.
	// It'll be the replacement for the `user.parse_identity(q.created_by)` in Py.
	// https://source.chromium.org/chromium/infra/infra/+/master:appengine/cr-buildbucket/search.py;l=294

	if !buildid.MayContainBuilds(q.StartTime, q.EndTime) {
		return &pb.SearchBuildsResponse{}, nil
	}

	// Validate bucket ACL permission.
	if q.BucketId != "" {
		index := strings.Index(q.BucketId, "/")
		if index <= 0 {
			return nil, appstatus.BadRequest(errors.Reason("invalid bucketId").Err())
		}
		if err := canRead(ctx, q.BucketId[:index], q.BucketId[index + 1:]); err != nil {
			return nil, err
		}
	}

	// Determine which subflow - directly query on Builds or on TagIndex
	isTagIndexCursor := tagIndexCursorRegex.MatchString(q.StartCursor)
	canUseTagIndex := len(indexed_tags(q.Tags)) != 0 &&
		(q.StartCursor == "" || isTagIndexCursor)
	if isTagIndexCursor && !canUseTagIndex {
		return nil, appstatus.BadRequest(errors.Reason("invalid cursor").Err())
	}
	if canUseTagIndex {
		// TODO(crbug/1090540): test switch-case block after tagIndexSearch() is complete.
		logging.Debugf(ctx, "Searching builds on TagIndex")
		switch res, err := tagIndexSearch(ctx, q); {
		case dbmodel.TagIndexIncomplete.In(err) && q.StartCursor == "":
			logging.Infof(ctx, "Falling back to querying search on builds.")
		case err != nil:
			return nil, err
		default:
			return res, nil
		}
	}

	// TODO(crbug/1090540): implement the function querySearch().
	return nil, nil
}

func indexed_tags(tags []string) []string{
	indexed := []string{}
	set := map[string]bool{}
	for _, tag := range tags {
		if !(strings.HasPrefix(tag, "buildset:") ||
			strings.HasPrefix(tag, "build_address:")) ||
			set[tag] {
			continue
		}
		indexed = append(indexed, tag)
		set[tag] = true
	}
	sort.Strings(indexed)
	return indexed
}


// TODO(crbug/1090540): implement search via tagIndex flow
func tagIndexSearch(ctx context.Context, q *imodel.SearchQuery) (*pb.SearchBuildsResponse, error) {
	logging.Debugf(ctx, "querying on TagIndex flow.")
	return nil, nil
}
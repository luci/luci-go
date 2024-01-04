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

package gerritchangelists

import (
	"context"
	"regexp"
	"strings"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/gerrit"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var (
	// Automation service accounts.
	automationAccountRE = regexp.MustCompile(`^.*@.*\.gserviceaccount\.com$`)
)

// LookupRequest represents parameters to a request to lookup up the
// owner kind of a gerrit changelist.
type LookupRequest struct {
	// Gerrit project in which the changelist is. Optional, but
	// gerrit prefers this be set to speed up lookups.
	GerritProject string
}

// FetchOwnerKinds retrieves the owner kind of each of the nominated
// gerrit changelists.
//
// For each changelist for which the owner kind is not cached in Spanner,
// this method will make an RPC to gerrit.
//
// This method must NOT be called within a Spanner transaction
// context, as it will create its own transactions to access
// the changelist cache.
func FetchOwnerKinds(ctx context.Context, reqs map[Key]LookupRequest) (map[Key]pb.ChangelistOwnerKind, error) {
	cacheResult := make(map[Key]*GerritChangelist)
	if len(reqs) > 0 {
		keys := make(map[Key]struct{})
		for key := range reqs {
			keys[key] = struct{}{}
		}

		// Try to retrieve changelist details from the Spanner cache.
		var err error
		cacheResult, err = Read(span.Single(ctx), keys)
		if err != nil {
			return nil, errors.Annotate(err, "read changelist cache").Err()
		}
	}

	var ms []*spanner.Mutation
	for key, req := range reqs {
		if _, ok := cacheResult[key]; ok {
			continue
		}

		// Retrieve the changelist details from Gerrit.
		ownerKind, err := retrieveChangelistOwnerKind(ctx, key, req.GerritProject)
		if err != nil {
			return nil, errors.Annotate(err, "retrieve owner kind from gerrit").Err()
		}
		cl := &GerritChangelist{
			Project:   key.Project,
			Host:      key.Host,
			Change:    key.Change,
			OwnerKind: ownerKind,
		}

		// Prepare a mutation to create/replace the Spanner cache entry for
		// this changelist. (Replacement may occur if multiple calls to
		// this method for the same changelist race.)
		m, err := CreateOrUpdate(cl)
		if err != nil {
			return nil, err
		}
		ms = append(ms, m)

		// Combine the fetched changelist details with those retrieved
		// from the cache earlier.
		cacheResult[key] = cl
	}

	if len(ms) > 0 {
		// Apply pending updates to the cache.
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			span.BufferWrite(ctx, ms...)
			return nil
		})
		if err != nil {
			return nil, errors.Annotate(err, "update changelist cache").Err()
		}
	}

	result := make(map[Key]pb.ChangelistOwnerKind)
	for key, entry := range cacheResult {
		result[key] = entry.OwnerKind
	}
	return result, nil
}

// PopulateOwnerKinds augments the given sources information to include
// the owner kind of each changelist.
//
// For each changelist for which the owner kind is not cached in Spanner,
// this method will make an RPC to gerrit.
//
// This method must NOT be called within a Spanner transaction
// context, as it will create its own transactions to access
// the changelist cache.
func PopulateOwnerKinds(ctx context.Context, project string, sourcesByID map[string]*rdbpb.Sources) (map[string]*pb.Sources, error) {
	if sourcesByID == nil {
		return make(map[string]*pb.Sources), nil
	}

	// Create the lookup requests.
	reqs := make(map[Key]LookupRequest)
	for _, sources := range sourcesByID {
		for _, cl := range sources.Changelists {
			key := Key{
				Project: project,
				Host:    cl.Host,
				Change:  cl.Change,
			}
			reqs[key] = LookupRequest{
				GerritProject: cl.Project,
			}
		}
	}

	kinds, err := FetchOwnerKinds(ctx, reqs)
	if err != nil {
		return nil, err
	}

	// Augmenting the original ResultDB sources with the owner kind
	// of each changelist, as retrieved from gerrit or the cache.
	result := make(map[string]*pb.Sources)
	for id, sources := range sourcesByID {
		augmentedSources := pbutil.SourcesFromResultDB(sources)

		for _, augmentedCL := range augmentedSources.Changelists {
			key := Key{
				Project: project,
				Host:    augmentedCL.Host,
				Change:  augmentedCL.Change,
			}

			// Augment each changelist with the owner kind.
			augmentedCL.OwnerKind = kinds[key]
		}
		result[id] = augmentedSources
	}
	return result, nil
}

// retrieveChangelistOwnerKind retrieves the owner kind for the
// given CL, using the given gerrit project hint (e.g. "chromium/src").
func retrieveChangelistOwnerKind(ctx context.Context, clKey Key, gerritProjectHint string) (pb.ChangelistOwnerKind, error) {
	if !strings.HasSuffix(clKey.Host, "-review.googlesource.com") {
		// Do not try and retrieve CL information from a gerrit host other
		// than those hosted on .googlesource.com. The CL hostname
		// could come from an untrusted source, and we don't want to leak
		// our authentication tokens to arbitrary hosts on the internet.
		return pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED, nil
	}

	client, err := gerrit.NewClient(ctx, clKey.Host, clKey.Project)
	if err != nil {
		return pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED, err
	}
	req := &gerritpb.GetChangeRequest{
		Number: clKey.Change,
		Options: []gerritpb.QueryOption{
			gerritpb.QueryOption_DETAILED_ACCOUNTS,
		},
		// Project hint, e.g. "chromium/src".
		// Reduces work on Gerrit server side.
		Project: gerritProjectHint,
	}
	fullChange, err := client.GetChange(ctx, req)
	code := status.Code(err)
	if code == codes.NotFound {
		logging.Warningf(ctx, "Changelist %s/%v for project %s not found.",
			clKey.Host, clKey.Change, clKey.Project)
		return pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED, nil
	}
	if code == codes.PermissionDenied {
		logging.Warningf(ctx, "LUCI Analysis does not have permission to read changelist %s/%v for project %s.",
			clKey.Host, clKey.Change, clKey.Project)
		return pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED, nil
	}
	if err != nil {
		return pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED, err
	}
	ownerEmail := fullChange.Owner.GetEmail()
	if automationAccountRE.MatchString(ownerEmail) {
		return pb.ChangelistOwnerKind_AUTOMATION, nil
	} else if ownerEmail != "" {
		return pb.ChangelistOwnerKind_HUMAN, nil
	}
	return pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED, nil
}

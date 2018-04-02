// Copyright 2017 The LUCI Authors.
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

package repo

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/metadata"
)

// Public returns publicly exposed implementation of cipd.Repository service.
//
// It checks ACLs.
func Public() api.RepositoryServer {
	return &repoImpl{meta: metadata.GetStorage()}
}

// repoImpl implements api.RepositoryServer.
type repoImpl struct {
	meta metadata.Storage
}

////////////////////////////////////////////////////////////////////////////////
// Prefix metadata RPC methods + related helpers.

// GetPrefixMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) GetPrefixMetadata(c context.Context, r *api.PrefixRequest) (resp *api.PrefixMetadata, err error) {
	// It is fine to implement this in terms of GetInheritedPrefixMetadata, since
	// we need to fetch all inherited metadata anyway to check ACLs.
	inherited, err := impl.GetInheritedPrefixMetadata(c, r)
	if err != nil {
		return nil, err
	}
	// Have the metadata for the requested prefix? It should be the last if so.
	if m := inherited.PerPrefixMetadata; len(m) != 0 && m[len(m)-1].Prefix == r.Prefix {
		return m[len(m)-1], nil
	}
	// Note that GetInheritedPrefixMetadata checked that the caller has permission
	// to view the requested prefix (via some parent prefix ACL), so sincerely
	// reply with NotFound.
	return nil, noMetadataErr(r.Prefix)
}

// GetInheritedPrefixMetadata implements the corresponding RPC method, see the
// proto doc.
//
// Note: it normalizes Prefix field inside the request.
func (impl *repoImpl) GetInheritedPrefixMetadata(c context.Context, r *api.PrefixRequest) (resp *api.InheritedPrefixMetadata, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(c, err) }()

	r.Prefix, err = common.ValidatePackagePrefix(r.Prefix)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'prefix' - %s", err)
	}

	metas, err := impl.checkRole(c, r.Prefix, api.Role_OWNER)
	if err != nil {
		return nil, err
	}
	return &api.InheritedPrefixMetadata{PerPrefixMetadata: metas}, nil
}

// UpdatePrefixMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) UpdatePrefixMetadata(c context.Context, r *api.PrefixMetadata) (resp *api.PrefixMetadata, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(c, err) }()

	// Fill in server-assigned fields.
	r.UpdateTime = google.NewTimestamp(clock.Now(c))
	r.UpdateUser = string(auth.CurrentIdentity(c))

	// Normalize and validate format of the PrefixMetadata.
	if err := common.NormalizePrefixMetadata(r); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad prefix metadata - %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(c, r.Prefix, api.Role_OWNER); err != nil {
		return nil, err
	}

	// Transactionally check the fingerprint and update the metadata. impl.meta
	// will recalculate the new fingerprint. Note there's a small chance the
	// caller no longer has OWNER role to modify the metadata inside the
	// transaction. We ignore it. It happens when caller's permissions are revoked
	// by someone else exactly during UpdatePrefixMetadata call.
	return impl.meta.UpdateMetadata(c, r.Prefix, func(cur *api.PrefixMetadata) error {
		if cur.Fingerprint != r.Fingerprint {
			switch {
			case cur.Fingerprint == "":
				// The metadata was deleted while the caller was messing with it.
				return noMetadataErr(r.Prefix)
			case r.Fingerprint == "":
				// Caller tries to make a new one, but we already have it.
				return status.Errorf(
					codes.AlreadyExists, "metadata for prefix %q already exists and has fingerprint %q, "+
						"use combination of GetPrefixMetadata and UpdatePrefixMetadata to "+
						"update it", r.Prefix, cur.Fingerprint)
			default:
				// The fingerprint has changed while the caller was messing with
				// the metadata.
				return status.Errorf(
					codes.FailedPrecondition, "metadata for prefix %q was updated concurrently "+
						"(the fingerprint in the request %q doesn't match the current fingerprint %q), "+
						"fetch new metadata with GetPrefixMetadata and reapply your "+
						"changes", r.Prefix, r.Fingerprint, cur.Fingerprint)
			}
		}
		*cur = *r
		return nil
	})
}

// checkRole checks where the caller has the given role in the given prefix or
// any of its parent prefixes.
//
// Understands role inheritance. See acl.go for more details.
//
// Returns grpc PermissionDenied error if the caller is not an owner. Fetches
// and returns metadata of the prefix and all parent prefixes as a side effect.
func (impl *repoImpl) checkRole(c context.Context, prefix string, role api.Role) ([]*api.PrefixMetadata, error) {
	metas, err := impl.meta.GetMetadata(c, prefix)
	if err != nil {
		return nil, err
	}
	switch yes, err := hasRole(c, metas, role); {
	case err != nil:
		return nil, err
	case !yes:
		return nil, metadataDeniedErr(prefix)
	default:
		return metas, nil
	}
}

// metadataDeniedErr produces a grpc error saying that the given prefix doesn't
// exist or the caller has no access to it.
func metadataDeniedErr(prefix string) error {
	return status.Errorf(codes.PermissionDenied, "prefix %q has no metadata or the caller is not allowed to see it", prefix)
}

// noMetadataErr produces a grpc error saying that the given prefix doesn't have
// metadata attached.
func noMetadataErr(prefix string) error {
	return status.Errorf(codes.NotFound, "prefix %q has no metadata", prefix)
}

// Copyright 2018 The LUCI Authors.
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

package metadata

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

// legacyStorageImpl implements Storage on top of PackageACL entities inherited
// from Python version of CIPD backend.
//
// This implementation stores api.PrefixMetadata in a deconstructed form as a
// bunch of datastore entities to be compatible with Python version of the
// backend that has no idea about PrefixMetadata abstraction.
//
// The processes of constructing and deconstructing PrefixMetadata are not
// perfectly reversible:
//   * Order of ACL entries in PrefixMetadata is not preserved.
//   * Order of principals in ACLs is also not preserved.
//   * Empty ACLs are removed from PrefixMetadata.
//
// These differences shouldn't have semantic importance for users through.
type legacyStorageImpl struct {
}

// GetMetadata is part of Storage interface.
//
// For each subprefix in 'prefix' (e.g. for "a/b/c" it would be "a", "a/b", ...)
// it fetches a bunch of packageACL entities (one per possible role), and merges
// them into single PrefixMetadata object, thus faking new metadata API on top
// of legacy entities.
//
// The result also always includes the hardcoded root metadata.
func (legacyStorageImpl) GetMetadata(c context.Context, prefix string) ([]*api.PrefixMetadata, error) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return nil, errors.Annotate(err, "bad prefix given to GetMetadata").Err()
	}

	// Grab all subprefixes, i.e. ["a", "a/b", "a/b/c"]
	var pfxs []string
	for i, ch := range prefix {
		if ch == '/' {
			pfxs = append(pfxs, prefix[:i])
		}
	}
	pfxs = append(pfxs, prefix)

	// Start with the root metadata.
	out := make([]*api.PrefixMetadata, 0, len(pfxs)+1)
	out = append(out, rootMetadata())

	// And finish with it if nothing else is requested.
	if len(pfxs) == 0 {
		return out, nil
	}

	// Prepare the keys.
	ents := make([]*packageACL, 0, len(pfxs)*len(legacyRoles))
	for _, p := range pfxs {
		ents = prefixACLs(c, p, ents)
	}

	// Fetch everything. ErrNoSuchEntity errors are fine, everything else is not.
	if err = datastore.Get(c, ents); isInternalDSError(err) {
		return nil, errors.Annotate(err, "datastore error when fetching PackageACL").Tag(transient.Tag).Err()
	}

	// Combine the result into a bunch of PrefixMetadata structs.
	legLen := len(legacyRoles)
	for i, pfx := range pfxs {
		if md := mergeIntoPrefixMetadata(c, pfx, ents[i*legLen:(i+1)*legLen]); md != nil {
			out = append(out, md)
		}
	}
	return out, nil
}

// VisitMetadata is part of Storage interface.
func (legacyStorageImpl) VisitMetadata(c context.Context, prefix string, cb Visitor) error {
	return fmt.Errorf("not implemented yet")
}

// UpdateMetadata is part of Storage interface.
//
// It assembles prefix metadata from a bunch of packageACL entities, passes it
// to the callback for modification, then deconstructs it back into a bunch of
// packageACL entities, to be saved in the datastore. All done transactionally.
func (legacyStorageImpl) UpdateMetadata(c context.Context, prefix string, cb func(m *api.PrefixMetadata) error) (*api.PrefixMetadata, error) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return nil, errors.Annotate(err, "bad prefix given to GetMetadata").Err()
	}
	if prefix == "" {
		return nil, errors.Reason("the root metadata is not modifiable").Err()
	}

	var cbErr error                 // error from 'cb'
	var updated *api.PrefixMetadata // updated metadata to return

	err = datastore.RunInTransaction(c, func(c context.Context) error {
		cbErr = nil // reset in case the transaction is being retried
		updated = nil

		// Fetch the existing metadata.
		ents := prefixACLs(c, prefix, nil)
		if err := datastore.Get(c, ents); isInternalDSError(err) {
			return errors.Annotate(err, "datastore error when fetching PackageACL").Err()
		}

		// Convert it to PrefixMetadata object. This will be nil if there's no
		// existing metadata, in which case we construct the default metadata
		// with no fingerprint (to indicate it is new), see UpdateMetadata doc in
		// Storage interface.
		updated = mergeIntoPrefixMetadata(c, prefix, ents)
		if updated == nil {
			updated = &api.PrefixMetadata{Prefix: prefix}
		}

		// Let the callback update the metadata. Retain the old copy for diff later.
		before := proto.Clone(updated).(*api.PrefixMetadata)
		if cbErr = cb(updated); cbErr != nil {
			return cbErr
		}

		// Don't let the callback mess with the prefix or the fingerprint.
		updated.Prefix = before.Prefix
		updated.Fingerprint = before.Fingerprint
		if proto.Equal(before, updated) {
			return nil // no changes whatsoever, don't touch anything
		}

		// Apply changes to the datastore. This updates 'ents' to match the metadata
		// stored in 'updated'. We then rederive PrefixMetadata (including the new
		// fingerprint) from them. We do it this way (instead of calculating the
		// fingerprint using 'updated' directly), to be absolutely sure that the
		// fingerprint returned by GetMetadata after this transaction lands matches
		// the fingerprint we return from UpdateMetadata. In particular, the way
		// we store ACLs in legacy entities doesn't preserve order of Acls entries
		// in the proto, or order of principals inside Acls, so we need to
		// "reformat" the updated metadata before calculating its fingerprint.
		if err := applyACLDiff(c, ents, updated); err != nil {
			return errors.Annotate(err, "failed to update PackageACL entities").Err()
		}
		updated = mergeIntoPrefixMetadata(c, prefix, ents)
		return nil
	}, nil)

	switch {
	case cbErr != nil:
		// The callback itself failed, need to return the error as is, as promised.
		return nil, cbErr
	case err != nil:
		// All other errors are from the datastore, consider them transient.
		return nil, errors.Annotate(err, "transaction failed").Tag(transient.Tag).Err()
	case updated == nil || updated.Fingerprint == "":
		// This happens if there's no existing metadata and the callback didn't
		// create it. Return nil to indicate that the metadata is still missing.
		return nil, nil
	}
	return updated, nil
}

////////////////////////////////////////////////////////////////////////////////
// Legacy entities with fields matching Python version of the CIPD backend.

// legacyRoles contain all roles that can show up as part of packageACL key.
//
// There's also COUNTER_WRITER legacy role that we skip: counters are
// deprecated.
var legacyRoles = []string{
	"OWNER",
	"WRITER",
	"READER",
}

// legacyRoleMap is mapping from legacy role names to non-legacy ones.
//
// Note: we can't use keys of this map instead of legacyRoles slice, since we
// always want to enumerate roles in order.
var legacyRoleMap = map[string]api.Role{
	"OWNER":  api.Role_OWNER,
	"WRITER": api.Role_WRITER,
	"READER": api.Role_READER,
}

// packageACL contains ACLs for some package prefix.
type packageACL struct {
	_kind  string                `gae:"$kind,PackageACL"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	ID     string         `gae:"$id"`         // "<ROLE>:<pkg/path>"
	Parent *datastore.Key `gae:"$parent"`     // see rootKey()
	Rev    int            `gae:"rev,noindex"` // incremented on each change

	Users      []string  `gae:"users,noindex"`
	Groups     []string  `gae:"groups,noindex"`
	ModifiedBy string    `gae:"modified_by"`
	ModifiedTS time.Time `gae:"modified_ts"`
}

// isInternalDSError is true for datastore errors that aren't entirely
// ErrNoSuchEntity.
func isInternalDSError(err error) bool {
	if err == nil {
		return false
	}

	merr, _ := err.(errors.MultiError)
	if merr == nil {
		return true // an overall RPC error
	}

	for _, e := range merr {
		if e != nil && e != datastore.ErrNoSuchEntity {
			return true // some serious issues with this single entity
		}
	}

	return false // all suberrors are either nil or ErrNoSuchEntity
}

// rootMetadata returns metadata of the root prefix.
//
// It is inherited by all prefixes.
func rootMetadata() *api.PrefixMetadata {
	// Administrators group has implicit permissions to do everything everywhere.
	//
	// TODO(vadimsh): Make this configurable.
	return &api.PrefixMetadata{
		Acls: []*api.PrefixMetadata_ACL{
			{
				Role:       api.Role_OWNER,
				Principals: []string{"group:administrators"},
			},
		},
	}
}

// rootKey returns a key of the root entity that stores ACL hierarchy.
func rootKey(c context.Context) *datastore.Key {
	return datastore.NewKey(c, "PackageACLRoot", "acls", 0, nil)
}

// prefixACLs appends empty packageACL entities (with keys populated) to the
// given slice and returns the new slice.
//
// The added entities have keys '<role>:<prefix>' where <role> goes over all
// possible roles. Adds exactly len(legacyRoles) entities.
func prefixACLs(c context.Context, prefix string, in []*packageACL) []*packageACL {
	root := rootKey(c)
	for _, r := range legacyRoles {
		in = append(in, &packageACL{
			ID:     r + ":" + prefix,
			Parent: root,
		})
	}
	return in
}

// mergeIntoPrefixMetadata takes exactly len(legacyRoles) entities and combines
// them into a single PrefixMetadata object if at least one of the entities is
// not empty. Returns nil of all entities are empty.
//
// The entities are expected to have IDs matching ones generated by
// prefixACLs(c, prefix). Panics otherwise.
//
// Logs and skips invalid principal names (should not be happening in reality).
func mergeIntoPrefixMetadata(c context.Context, prefix string, ents []*packageACL) *api.PrefixMetadata {
	if len(ents) != len(legacyRoles) {
		panic(fmt.Sprintf("expecting %d entities, got %d", len(legacyRoles), len(ents)))
	}

	modTime := time.Time{}                   // max(ent.ModifiedTS)
	md := api.PrefixMetadata{Prefix: prefix} // to be returned if not empty

	for i, r := range legacyRoles {
		pkgACL := ents[i] // an instance of *packageACL
		if expectedID := r + ":" + prefix; pkgACL.ID != expectedID {
			panic(fmt.Sprintf("expecting key %q, got %q", expectedID, pkgACL.ID))
		}

		if pkgACL.ModifiedTS.IsZero() {
			continue // zero body, no such entity in the datastore, skip
		}

		// Detect the most recently modified packageACL to use its modification time
		// as overall update time of PrefixMetadata object.
		if modTime.IsZero() || modTime.Before(pkgACL.ModifiedTS) {
			modTime = pkgACL.ModifiedTS
			md.UpdateUser = pkgACL.ModifiedBy
			md.UpdateTime = google.NewTimestamp(modTime)
		}

		// Collect a list of principals defined in packageACL, skipping unrecognized
		// ones.
		principals := make([]string, 0, len(pkgACL.Users)+len(pkgACL.Groups))
		for _, u := range pkgACL.Users {
			if _, err := identity.MakeIdentity(u); err != nil {
				logging.Errorf(c, "Bad identity %q in PackageACL %q - %s", u, pkgACL.ID, err)
			} else {
				principals = append(principals, u)
			}
		}
		for _, g := range pkgACL.Groups {
			principals = append(principals, "group:"+g)
		}

		if len(principals) != 0 {
			md.Acls = append(md.Acls, &api.PrefixMetadata_ACL{
				Role:       legacyRoleMap[r],
				Principals: principals,
			})
		}
	}

	// If modTime is zero, then none of packageACL entities exist. Return nil in
	// this case to indicate there's no metadata at all. Note that it is possible
	// that some packageACL entities exist, but have empty ACLs. We consider this
	// as NOT empty metadata (we still have modification time and modifying user
	// information). Thus we do not check len(md.Acls) == 0 here. Callers will
	// see PrefixMetadata{...} with empty ACLs field, this should be expected.
	if modTime.IsZero() {
		return nil
	}

	// Calculate the fingerprint now that we have assembled everything.
	md.Fingerprint = CalculateFingerprint(md)
	return &md
}

// applyACLDiff extracts ACLs from 'md', compares them to 'ents', and Puts all
// updated entities into the datastore, updating 'ents'.
//
// In the end 'ents' together contain the new updated metadata.
//
// 'ents' are expected to have len(legacyRoles) entries, ordered by legacyRoles
// roles. Panics otherwise.
func applyACLDiff(c context.Context, ents []*packageACL, md *api.PrefixMetadata) error {
	if len(ents) != len(legacyRoles) {
		panic(fmt.Sprintf("expecting %d entities, got %d", len(legacyRoles), len(ents)))
	}

	// Convert md.ACLs to a map role -> principals, for easier access.
	perRole := make(map[api.Role][]string, len(md.Acls))
	for _, acl := range md.Acls {
		perRole[acl.Role] = acl.Principals
	}

	// Entities to put into the datastore.
	toPut := []*packageACL{}

	for i, r := range legacyRoles {
		oldACL := ents[i] // an instance of *packageACL
		if expectedID := r + ":" + md.Prefix; oldACL.ID != expectedID {
			panic(fmt.Sprintf("expecting key %q, got %q", expectedID, oldACL.ID))
		}

		// Grab Users and Group from the updated metadata (in 'md') to compare them
		// to what's in the oldACL.
		users := make([]string, 0, len(oldACL.Users))
		groups := make([]string, 0, len(oldACL.Groups))
		for _, pr := range perRole[legacyRoleMap[r]] {
			if strings.HasPrefix(pr, "group:") {
				groups = append(groups, strings.TrimPrefix(pr, "group:"))
			} else {
				users = append(users, pr)
			}
		}
		if isEqualStrSlice(users, oldACL.Users) && isEqualStrSlice(groups, oldACL.Groups) {
			continue // no changes, do not touch this entity
		}

		// This ACL was modified! Update it.
		oldACL.Rev++
		oldACL.Users = users
		oldACL.Groups = groups
		oldACL.ModifiedBy = md.UpdateUser
		oldACL.ModifiedTS = google.TimeFromProto(md.UpdateTime)
		toPut = append(toPut, oldACL)
	}

	return datastore.Put(c, toPut)
}

func isEqualStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

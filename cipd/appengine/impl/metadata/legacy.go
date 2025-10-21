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
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common"
)

// rootMeta is metadata of the root prefix, it is inherited by all prefixes.
//
// TODO(vadimsh): Make this configurable.
var rootMeta = &repopb.PrefixMetadata{
	Acls: []*repopb.PrefixMetadata_ACL{
		// Administrators have implicit permissions to do everything everywhere.
		{
			Role:       repopb.Role_OWNER,
			Principals: []string{"group:administrators"},
		},
	},
}

func init() {
	CalculateFingerprint(rootMeta)
}

// legacyStorageImpl implements Storage on top of PackageACL entities inherited
// from Python version of CIPD backend.
//
// This implementation stores repopb.PrefixMetadata in a deconstructed form as a
// bunch of datastore entities to be compatible with Python version of the
// backend that has no idea about PrefixMetadata abstraction.
//
// The processes of constructing and deconstructing PrefixMetadata are not
// perfectly reversible:
//   - Order of ACL entries in PrefixMetadata is not preserved.
//   - Order of principals in ACLs is also not preserved.
//   - Empty ACLs are removed from PrefixMetadata.
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
func (legacyStorageImpl) GetMetadata(ctx context.Context, prefix string) ([]*repopb.PrefixMetadata, error) {
	md, _, err := getMetadataImpl(ctx, prefix)
	return md, err
}

// getMetadataImpl implements GetMetadata.
//
// As a bonus it returns all packageACL entities it fetched. This is used by
// VisitMetadata to avoid unnecessary refetches.
func getMetadataImpl(ctx context.Context, prefix string) ([]*repopb.PrefixMetadata, []*packageACL, error) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return nil, nil, errors.Fmt("bad prefix given to GetMetadata: %w", err)
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
	out := make([]*repopb.PrefixMetadata, 0, len(pfxs)+1)
	out = append(out, rootMetadata())

	// And finish with it if nothing else is requested.
	if len(pfxs) == 0 {
		return out, nil, nil
	}

	// Prepare the keys.
	ents := make([]*packageACL, 0, len(pfxs)*len(legacyRoles))
	for _, p := range pfxs {
		ents = prefixACLs(ctx, p, ents)
	}

	// Fetch everything. ErrNoSuchEntity errors are fine, everything else is not.
	if err = datastore.Get(ctx, ents); isInternalDSError(err) {
		return nil, nil, transient.Tag.Apply(errors.Fmt("datastore error when fetching PackageACL: %w", err))
	}

	// Combine the result into a bunch of PrefixMetadata structs.
	legLen := len(legacyRoles)
	for i, pfx := range pfxs {
		if md := mergeIntoPrefixMetadata(ctx, pfx, ents[i*legLen:(i+1)*legLen]); md != nil {
			out = append(out, md)
		}
	}
	return out, ents, nil
}

// VisitMetadata is part of Storage interface.
func (legacyStorageImpl) VisitMetadata(ctx context.Context, prefix string, cb Visitor) error {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return errors.Fmt("bad prefix given to VisitMetadata: %w", err)
	}

	// Visit 'prefix' directly first, per VisitMetadata contract. There's a chance
	// we won't need to recurse deeper at all and can skip all expensive fetches.
	md, ents, err := getMetadataImpl(ctx, prefix)
	if err != nil {
		return err
	}
	switch cont, err := cb(prefix, md); {
	case err != nil:
		return err
	case !cont:
		return nil
	}

	// We'll have to recurse into metadata subtree after all. Unfortunately,
	// with legacy entity structure there's no way to efficiently fetch only
	// immediate children of 'prefix', so we fetch EVERYTHING in advance, building
	// a metadata graph for prefix/... in memory.
	gr := metadataGraph{}
	gr.init(rootMetadata())

	// Seed this graph with already fetched entities. They'll be needed to derive
	// metadata inherited from 'prefix' and its parents when visiting nodes. For
	// example, the metadata graph when visiting prefix "a/b" may look like this:
	//
	//                     - "a/b/c"
	//                    /
	//  ROOT - "a"- "a/b" -- "a/b/d"
	//                    \
	//                     - "a/b/e"
	//
	// The traversal will be started form "a/b", but we still need the nodes
	// leading to the root to get all inherited metadata.
	gr.insert(ctx, ents)

	// Fetch each per-role subtree separately, they have different key prefixes.
	err = parallel.FanOutIn(func(tasks chan<- func() error) {
		mu := sync.Mutex{}
		for _, role := range legacyRoles {
			tasks <- func() error {
				listing, err := listACLsByPrefix(ctx, role, prefix)
				if err == nil {
					mu.Lock()
					gr.insert(ctx, listing)
					mu.Unlock()
				}
				return err
			}
		}
	})
	if err != nil {
		return transient.Tag.Apply(errors.
			Fmt("failed to fetch metadata: %w", err))
	}

	// Make sure we have a path to 'prefix' before we freeze the graph. We need it
	// to start the traversal below. It may be missing if there's no metadata
	// attached to it and it has no children. This will naturally be handled by
	// 'traverse'.
	pfx := gr.node(prefix)

	// Calculate all PrefixMetadata entries in the graph.
	gr.freeze(ctx)

	// Traverse the graph, but make sure to skip 'prefix' itself, we've already
	// visited it at the very beginning.
	return pfx.traverse(nil, func(n *metadataNode, md []*repopb.PrefixMetadata) (cont bool, err error) {
		switch {
		case n.prefix == prefix:
			return true, nil
		case n.md == nil:
			return true, nil // an intermediary node with no metadata, look deeper
		default:
			return cb(n.prefix, md)
		}
	})
}

// UpdateMetadata is part of Storage interface.
//
// It assembles prefix metadata from a bunch of packageACL entities, passes it
// to the callback for modification, then deconstructs it back into a bunch of
// packageACL entities, to be saved in the datastore. All done transactionally.
func (legacyStorageImpl) UpdateMetadata(ctx context.Context, prefix string, cb func(ctx context.Context, m *repopb.PrefixMetadata) error) (*repopb.PrefixMetadata, error) {
	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return nil, errors.Fmt("bad prefix given to GetMetadata: %w", err)
	}
	if prefix == "" {
		return nil, errors.New("the root metadata is not modifiable")
	}

	var cbErr error                    // error from 'cb'
	var updated *repopb.PrefixMetadata // updated metadata to return

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		cbErr = nil // reset in case the transaction is being retried
		updated = nil

		// Fetch the existing metadata.
		ents := prefixACLs(ctx, prefix, nil)
		if err := datastore.Get(ctx, ents); isInternalDSError(err) {
			return errors.Fmt("datastore error when fetching PackageACL: %w", err)
		}

		// Convert it to PrefixMetadata object. This will be nil if there's no
		// existing metadata, in which case we construct the default metadata
		// with no fingerprint (to indicate it is new), see UpdateMetadata doc in
		// Storage interface.
		updated = mergeIntoPrefixMetadata(ctx, prefix, ents)
		if updated == nil {
			updated = &repopb.PrefixMetadata{Prefix: prefix}
		}

		// Let the callback update the metadata. Retain the old copy for diff later.
		before := proto.Clone(updated).(*repopb.PrefixMetadata)
		if cbErr = cb(ctx, updated); cbErr != nil {
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
		if err := applyACLDiff(ctx, ents, updated); err != nil {
			return errors.Fmt("failed to update PackageACL entities: %w", err)
		}
		updated = mergeIntoPrefixMetadata(ctx, prefix, ents)
		return nil
	}, nil)

	switch {
	case cbErr != nil:
		// The callback itself failed, need to return the error as is, as promised.
		return nil, cbErr
	case err != nil:
		// All other errors are from the datastore, consider them transient.
		return nil, transient.Tag.Apply(errors.Fmt("transaction failed: %w", err))
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
var legacyRoleMap = map[string]repopb.Role{
	"OWNER":  repopb.Role_OWNER,
	"WRITER": repopb.Role_WRITER,
	"READER": repopb.Role_READER,
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

// parseKey parses the entity key into its components, validating them.
//
// On success, role is one of legacyRoles and prefix is non-empty valid prefix.
func (e *packageACL) parseKey() (role, prefix string, err error) {
	chunks := strings.Split(e.ID, ":")
	if len(chunks) != 2 {
		return "", "", fmt.Errorf("invalid key %q, not <role>:<prefix> pair", e.ID)
	}
	role = chunks[0]
	if _, ok := legacyRoleMap[role]; !ok {
		return "", "", fmt.Errorf("unrecognized role in the key %q", e.ID)
	}
	prefix, err = common.ValidatePackagePrefix(chunks[1])
	if err != nil || prefix == "" { // note: there's no ACLs for root in the datastore
		return "", "", fmt.Errorf("invalid package prefix in the key %q", e.ID)
	}
	return
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

// rootMetadata returns metadata of the root prefix (always a new copy).
//
// It is inherited by all prefixes.
func rootMetadata() *repopb.PrefixMetadata {
	return proto.Clone(rootMeta).(*repopb.PrefixMetadata)
}

// rootKey returns a key of the root entity that stores ACL hierarchy.
func rootKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "PackageACLRoot", "acls", 0, nil)
}

// prefixACLs appends empty packageACL entities (with keys populated) to the
// given slice and returns the new slice.
//
// The added entities have keys '<role>:<prefix>' where <role> goes over all
// possible roles. Adds exactly len(legacyRoles) entities.
func prefixACLs(ctx context.Context, prefix string, in []*packageACL) []*packageACL {
	root := rootKey(ctx)
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
// prefixACLs(ctx, prefix). Panics otherwise.
//
// Logs and skips invalid principal names (should not be happening in reality).
func mergeIntoPrefixMetadata(ctx context.Context, prefix string, ents []*packageACL) *repopb.PrefixMetadata {
	if len(ents) != len(legacyRoles) {
		panic(fmt.Sprintf("expecting %d entities, got %d", len(legacyRoles), len(ents)))
	}

	modTime := time.Time{}                      // max(ent.ModifiedTS)
	md := repopb.PrefixMetadata{Prefix: prefix} // to be returned if not empty

	for i, r := range legacyRoles {
		pkgACL := ents[i]
		if pkgACL == nil {
			continue // not present, no ACL for role 'r'
		}

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
			md.UpdateTime = timestamppb.New(modTime)
		}

		// Collect a list of principals defined in packageACL, skipping unrecognized
		// ones.
		principals := make([]string, 0, len(pkgACL.Users)+len(pkgACL.Groups))
		for _, u := range pkgACL.Users {
			if _, err := identity.MakeIdentity(u); err != nil {
				logging.Errorf(ctx, "Bad identity %q in PackageACL %q - %s", u, pkgACL.ID, err)
			} else {
				principals = append(principals, u)
			}
		}
		for _, g := range pkgACL.Groups {
			principals = append(principals, "group:"+g)
		}

		if len(principals) != 0 {
			md.Acls = append(md.Acls, &repopb.PrefixMetadata_ACL{
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
	CalculateFingerprint(&md)
	return &md
}

// applyACLDiff extracts ACLs from 'md', compares them to 'ents', and Puts all
// updated entities into the datastore, updating 'ents'.
//
// In the end 'ents' together contain the new updated metadata.
//
// 'ents' are expected to have len(legacyRoles) entries, ordered by legacyRoles
// roles. Panics otherwise.
func applyACLDiff(ctx context.Context, ents []*packageACL, md *repopb.PrefixMetadata) error {
	if len(ents) != len(legacyRoles) {
		panic(fmt.Sprintf("expecting %d entities, got %d", len(legacyRoles), len(ents)))
	}

	// Convert md.ACLs to a map role -> principals, for easier access.
	perRole := make(map[repopb.Role][]string, len(md.Acls))
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
		oldACL.ModifiedTS = md.UpdateTime.AsTime()
		toPut = append(toPut, oldACL)
	}

	return datastore.Put(ctx, toPut)
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

// listACLsByPrefix returns packageACL entities with ACLs for the given legacy
// role under the given prefix.
//
// The prefix should be in a form produced by ValidatePackagePrefix, i.e. no
// trailing / and "" denotes the root. ACLs for the prefix itself are NOT
// listed. Only ACLs strictly underneath are.
//
// The return value is sorted by prefix.
func listACLsByPrefix(ctx context.Context, role, prefix string) (acls []*packageACL, err error) {
	if prefix, err = common.ValidatePackagePrefix(prefix); err != nil {
		return nil, err
	}

	keyPfx := role + ":"
	if prefix != "" {
		keyPfx += prefix + "/"
	}

	root := rootKey(ctx)

	// Note: __key__ queries are already ordered by key.
	q := datastore.NewQuery("PackageACL").Ancestor(root)
	q = q.Gt("__key__", datastore.KeyForObj(ctx, &packageACL{
		ID:     keyPfx + " ",
		Parent: root,
	}))
	q = q.Lt("__key__", datastore.KeyForObj(ctx, &packageACL{
		ID:     keyPfx + "~",
		Parent: root,
	}))

	if err = datastore.GetAll(ctx, q, &acls); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("failed to query the list of ACLs: %w", err))
	}
	return
}

////////////////////////////////////////////////////////////////////////////////
// Metadata graph used by VisitMetadata implementation.

// metadataNode is a single node in the metadata tree.
//
// It can be in non-frozen (== under construction) and frozen (== constructed)
// states.
type metadataNode struct {
	prefix string        // this node's full prefix, e.g. "a/b/c"
	acls   []*packageACL // exactly len(legacyRoles) items with node's ACLs, nil when frozen

	parent   *metadataNode
	children map[string]*metadataNode // direct children of the node

	// md is finalized metadata derived from 'acls' in 'freeze'.
	//
	// It may be nil for intermediary nodes that do not have metadata attached to
	// them.
	md *repopb.PrefixMetadata
}

// assertFrozen panics if the node is not frozen yet.
func (n *metadataNode) assertFrozen() {
	if n.acls != nil {
		panic("not frozen yet")
	}
}

// assertNonFrozen panics if the node is already frozen.
func (n *metadataNode) assertNonFrozen() {
	if n.acls == nil {
		panic("frozen already")
	}
}

// child returns a direct child node, creating it if necessary.
func (n *metadataNode) child(name string) *metadataNode {
	if c, ok := n.children[name]; ok {
		return c
	}
	n.assertNonFrozen()
	if n.children == nil {
		n.children = make(map[string]*metadataNode, 1)
	}
	c := &metadataNode{
		parent: n,
		acls:   make([]*packageACL, len(legacyRoles)),
	}
	if n.prefix == "" {
		c.prefix = name
	} else {
		c.prefix = n.prefix + "/" + name
	}
	n.children[name] = c
	return c
}

// attachACL attaches a packageACL to this node.
//
// All such attached ACLs are merged into single PrefixMetadata in 'freeze'.
func (n *metadataNode) attachACL(role string, e *packageACL) {
	n.assertNonFrozen()

	// 'acls' are ordered by legacyRoles. Insert 'e' into the corresponding slot.
	// Such ordering is required by mergeIntoPrefixMetadata used by 'freeze'.
	for i, r := range legacyRoles {
		if r == role {
			if n.acls[i] != nil {
				panic(fmt.Sprintf("metadata for role %q at %q is already attached", role, n.prefix))
			}
			n.acls[i] = e
			return
		}
	}

	// 'attachACL' is called only with roles validated by packageACL.parseKey(),
	// so this is impossible.
	panic(fmt.Sprintf("unexpected impossible role %q", role))
}

// freeze marks the node and its subtree as fully constructed, calculating their
// PrefixMetadata from attached ACLs.
//
// The context is used only for logging.
func (n *metadataNode) freeze(ctx context.Context) {
	n.assertNonFrozen()

	// md may be already non-nil for the root, this is fine.
	if n.md == nil {
		n.md = mergeIntoPrefixMetadata(ctx, n.prefix, n.acls)
	}
	n.acls = nil // mark as frozen, release unnecessary memory

	for _, child := range n.children {
		child.freeze(ctx)
	}
}

// metadata returns this node's and all inherited metadata.
//
// Root metadata first. The return value is never nil. If there's no metadata,
// returns non-nil empty slice.
func (n *metadataNode) metadata() (md []*repopb.PrefixMetadata) {
	n.assertFrozen()
	if n.parent != nil {
		md = n.parent.metadata()
	} else {
		md = make([]*repopb.PrefixMetadata, 0, 32) // 32 is picked arbitrarily
	}
	if n.md != nil {
		md = append(md, n.md)
	}
	return
}

// traverse does depth-first traversal of the node's subtree starting from self.
//
// 'md', if non-nil, is metadata from all previous parents (starting from the
// root). If it is nil, all inherited metadata will be calculated from scratch.
//
// Note that non-nil empty 'md' slice is a valid value (it means there's nothing
// to inherit), no recalculation will be done in this case.
//
// Children are visited in lexicographical order.
func (n *metadataNode) traverse(md []*repopb.PrefixMetadata, cb func(*metadataNode, []*repopb.PrefixMetadata) (bool, error)) error {
	n.assertFrozen()

	if md == nil {
		md = n.metadata() // calculate from scratch
	} else {
		if n.md != nil {
			md = append(md, n.md) // just extend what we have
		}
	}

	switch cont, err := cb(n, md); {
	case err != nil:
		return err
	case !cont:
		return nil
	}

	keys := make([]string, 0, len(n.children))
	for k := range n.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if err := n.children[k].traverse(md, cb); err != nil {
			return err
		}
	}
	return nil
}

// metadataGraph is a in-memory metadata graph used by VisitMetadata.
type metadataGraph struct {
	root *metadataNode // matches prefix ""
}

// init initializes the root node.
func (g *metadataGraph) init(root *repopb.PrefixMetadata) {
	if root != nil && root.Prefix != "" {
		panic("the root node metadata should have empty prefix")
	}
	g.root = &metadataNode{
		acls: make([]*packageACL, len(legacyRoles)),
		md:   root,
	}
}

// node returns a node at the given path, constructing it if necessary.
//
// 'path' here has a form "a/b/c", relative to the absolute root ("").
func (g *metadataGraph) node(path string) *metadataNode {
	cur := g.root
	if path != "" {
		for _, elem := range strings.Split(path, "/") {
			cur = cur.child(elem)
		}
	}
	return cur
}

// insert attaches ACL in the given entities to nodes in the graph.
//
// Silently ignores empty entities (based on ModifiedTS value). Logs and ignores
// broken ones. The context is used only for logging.
func (g *metadataGraph) insert(ctx context.Context, ents []*packageACL) {
	for _, e := range ents {
		if e.ModifiedTS.IsZero() {
			continue // zero body, no such entity in the datastore, skip
		}
		role, pfx, err := e.parseKey()
		if err != nil {
			logging.Errorf(ctx, "Skipping bad PackageACL entity - %s", err)
			continue
		}
		g.node(pfx).attachACL(role, e)
	}
}

// freeze finalizes graph construction by calculating all PrefixMetadata items.
//
// The graph is not modifiable after this point, but it becomes traversable.
// The context is used only for logging.
func (g *metadataGraph) freeze(ctx context.Context) {
	g.root.freeze(ctx)
}

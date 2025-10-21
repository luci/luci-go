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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	repogrpcpb "go.chromium.org/luci/cipd/api/cipd/v1/repopb/grpcpb"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/cipd/appengine/impl/metadata"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/repo/processing"
	"go.chromium.org/luci/cipd/appengine/impl/repo/tasks"
	"go.chromium.org/luci/cipd/appengine/impl/vsa"
	"go.chromium.org/luci/cipd/common"
)

// PrefixesViewers is a group membership in which grants read access to all
// prefix metadata regardless of individual prefix ACLs.
const PrefixesViewers = "cipd-prefixes-viewers"

// Public returns publicly exposed implementation of cipd.Repository service.
//
// It checks ACLs.
func Public(internalCAS cas.StorageServer, d *tq.Dispatcher, c vsa.Client) Server {
	impl := &repoImpl{
		tq:   d,
		meta: metadata.GetStorage(),
		cas:  internalCAS,
		vsa:  c,
	}
	impl.registerTasks()
	impl.registerProcessor(&processing.ClientExtractor{CAS: internalCAS})
	impl.registerProcessor(&processing.BootstrapPackageExtractor{CAS: internalCAS})
	return impl
}

// Server is repopb.RepositoryServer that can also expose some non-pRPC routes.
type Server interface {
	repogrpcpb.RepositoryServer

	// InstallHandlers installs non-pRPC HTTP handlers into the router.
	//
	// Assumes 'base' middleware chain does OAuth2 authentication already.
	InstallHandlers(r *router.Router, base router.MiddlewareChain)
}

// repoImpl implements repopb.RepositoryServer.
type repoImpl struct {
	repogrpcpb.UnimplementedRepositoryServer

	tq *tq.Dispatcher

	meta metadata.Storage  // storage for package prefix metadata
	cas  cas.StorageServer // non-ACLed storage for instance package files
	vsa  vsa.Client

	procs    []processing.Processor          // in order of registerProcessor calls
	procsMap map[string]processing.Processor // ID => processing.Processor
}

// registerTasks adds tasks to the tq Dispatcher.
func (impl *repoImpl) registerTasks() {
	// See queue.yaml for "run-processors" task queue definition.
	impl.tq.RegisterTaskClass(tq.TaskClass{
		ID:        "run-processors",
		Prototype: &tasks.RunProcessors{},
		Kind:      tq.Transactional,
		Queue:     "run-processors",
		Handler: func(ctx context.Context, m proto.Message) error {
			return impl.runProcessorsTask(ctx, m.(*tasks.RunProcessors))
		},
	})
	// See queue.yaml for "vsa-requests" task queue definition.
	impl.tq.RegisterTaskClass(tq.TaskClass{
		ID:        "vsa-requests",
		Prototype: &tasks.CallVerifySoftwareArtifact{},
		Kind:      tq.NonTransactional,
		Queue:     "vsa-requests",
		Handler: func(ctx context.Context, m proto.Message) error {
			return impl.callVerifySoftwareArtifact(ctx, m.(*tasks.CallVerifySoftwareArtifact))
		},
	})
}

// registerProcessor adds a new processor.
func (impl *repoImpl) registerProcessor(p processing.Processor) {
	if impl.procsMap == nil {
		impl.procsMap = map[string]processing.Processor{}
	}

	id := p.ID()
	if impl.procsMap[id] != nil {
		panic(fmt.Sprintf("processor %q has already been registered", id))
	}

	impl.procs = append(impl.procs, p)
	impl.procsMap[id] = p
}

// packageReader opens a package instance for reading.
func (impl *repoImpl) packageReader(ctx context.Context, ref *caspb.ObjectRef) (*processing.PackageReader, error) {
	// Get slow Google Storage based ReaderAt.
	rawReader, err := impl.cas.GetReader(ctx, ref)
	switch code := status.Code(err); {
	case code == codes.NotFound:
		return nil, errors.Fmt("package instance is not in the storage: %w", err)
	case code != codes.OK:
		return nil, transient.Tag.Apply(errors.Fmt("failed to open the object for reading: %w", err))
	}

	// Read in 512 Kb chunks, keep 2 of them buffered.
	pkg, err := processing.NewPackageReader(
		iotools.NewBufferingReaderAt(rawReader, 512*1024, 2),
		rawReader.Size())
	if err != nil {
		return nil, errors.Fmt("error when opening the package: %w", err)
	}
	return pkg, nil
}

////////////////////////////////////////////////////////////////////////////////
// Prefix metadata RPC methods + related helpers including ACL checks.

// GetPrefixMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) GetPrefixMetadata(ctx context.Context, r *repopb.PrefixRequest) (resp *repopb.PrefixMetadata, err error) {
	// It is fine to implement this in terms of GetInheritedPrefixMetadata, since
	// we need to fetch all inherited metadata anyway to check ACLs.
	inherited, err := impl.GetInheritedPrefixMetadata(ctx, r)
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

func (impl *repoImpl) canGetPrefixMetadata(ctx context.Context, prefix string) (metas []*repopb.PrefixMetadata, err error) {
	prefixViewer, err := auth.IsMember(ctx, PrefixesViewers)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check group membership")
	}
	if !prefixViewer {
		// Check if have OWNER access in the prefix, this also fetches metadata as
		// a side effect (it is needed to check ACLs too).
		metas, err = impl.checkRole(ctx, prefix, repopb.Role_OWNER)
	}
	return
}

// GetInheritedPrefixMetadata implements the corresponding RPC method, see the
// proto doc.
//
// Note: it normalizes Prefix field inside the request.
func (impl *repoImpl) GetInheritedPrefixMetadata(ctx context.Context, r *repopb.PrefixRequest) (resp *repopb.InheritedPrefixMetadata, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	r.Prefix, err = common.ValidatePackagePrefix(r.Prefix)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'prefix': %s", err)
	}

	metas, err := impl.canGetPrefixMetadata(ctx, r.Prefix)
	if err != nil {
		return nil, err
	}
	if metas == nil {
		metas, err = impl.meta.GetMetadata(ctx, r.Prefix)
	}

	return &repopb.InheritedPrefixMetadata{PerPrefixMetadata: metas}, nil
}

// UpdatePrefixMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) UpdatePrefixMetadata(ctx context.Context, r *repopb.PrefixMetadata) (resp *repopb.PrefixMetadata, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Fill in server-assigned fields.
	r.UpdateTime = timestamppb.New(clock.Now(ctx))
	r.UpdateUser = string(auth.CurrentIdentity(ctx))

	// Normalize and validate format of the PrefixMetadata.
	if err := common.NormalizePrefixMetadata(r); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad prefix metadata: %s", err)
	}

	// The root metadata is not modifiable through API.
	if r.Prefix == "" {
		return nil, status.Errorf(codes.InvalidArgument, "the root metadata is not modifiable")
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Prefix, repopb.Role_OWNER); err != nil {
		return nil, err
	}

	// Transactionally check the fingerprint and update the metadata. impl.meta
	// will recalculate the new fingerprint. Note there's a small chance the
	// caller no longer has OWNER role to modify the metadata inside the
	// transaction. We ignore it. It happens when caller's permissions are revoked
	// by someone else exactly during UpdatePrefixMetadata call.
	return impl.meta.UpdateMetadata(ctx, r.Prefix, func(ctx context.Context, cur *repopb.PrefixMetadata) error {
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
		prev := proto.Clone(cur).(*repopb.PrefixMetadata)
		proto.Reset(cur)
		proto.Merge(cur, r)
		return model.EmitMetadataEvents(ctx, prev, cur)
	})
}

// GetRolesInPrefix implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) GetRolesInPrefix(ctx context.Context, r *repopb.PrefixRequest) (resp *repopb.RolesInPrefixResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	return impl.getRolesInPrefixImpl(ctx, auth.CurrentIdentity(ctx), nil, r)
}

// GetRolesInPrefixOnBehalfOf implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) GetRolesInPrefixOnBehalfOf(ctx context.Context, r *repopb.PrefixRequestOnBehalfOf) (resp *repopb.RolesInPrefixResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	metas, err := impl.canGetPrefixMetadata(ctx, r.PrefixRequest.Prefix)
	if err != nil {
		return nil, err
	}

	ident, err := identity.MakeIdentity(r.Identity)
	if err != nil {
		return nil, err
	}
	switch ident.Kind() {
	case identity.Anonymous, identity.User:
		// These are the only allowed kinds.
	default:
		return nil, status.Errorf(codes.InvalidArgument, "identity must be of kind Anonymous or User.")
	}
	return impl.getRolesInPrefixImpl(ctx, ident, metas, r.PrefixRequest)
}

// Implements both GetRolesInPrefixOnBehalfOf and GetRolesInPrefix.
//
// `ident` must always be supplied - this function will validate it.
// `metas` is optional - in the case of GetRolesInPrefixOnBehalfOf, it may be
// fetched as a side-effect of checking permissions. In other cases, this
// function will fetch it directly without additional permission checks.
func (impl *repoImpl) getRolesInPrefixImpl(ctx context.Context, ident identity.Identity, metas []*repopb.PrefixMetadata, r *repopb.PrefixRequest) (resp *repopb.RolesInPrefixResponse, err error) {
	r.Prefix, err = common.ValidatePackagePrefix(r.Prefix)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'prefix': %s", err)
	}
	if err := ident.Validate(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'identity': %s", err)
	}

	if metas == nil {
		metas, err = impl.meta.GetMetadata(ctx, r.Prefix)
		if err != nil {
			return nil, err
		}
	}

	roles, err := rolesInPrefix(ctx, ident, metas)
	if err != nil {
		return nil, err
	}

	resp = &repopb.RolesInPrefixResponse{
		Roles: make([]*repopb.RolesInPrefixResponse_RoleInPrefix, len(roles)),
	}
	for i, r := range roles {
		resp.Roles[i] = &repopb.RolesInPrefixResponse_RoleInPrefix{Role: r}
	}
	return resp, nil
}

// checkRole checks where the caller has the given role in the given prefix or
// any of its parent prefixes.
//
// Understands role inheritance. See acl.go for more details.
//
// Returns grpc PermissionDenied error if the caller doesn't have the requested
// role. The error message depends on whether caller has READER role or not
// (readers see more details).
//
// Fetches and returns metadata of the prefix and all parent prefixes as a side
// effect.
func (impl *repoImpl) checkRole(ctx context.Context, prefix string, role repopb.Role) ([]*repopb.PrefixMetadata, error) {
	metas, err := impl.meta.GetMetadata(ctx, prefix)
	if err != nil {
		return nil, err
	}

	switch yes, err := hasRole(ctx, metas, role); {
	case err != nil:
		return nil, err
	case yes:
		return metas, nil
	case role == repopb.Role_READER: // was checking for a reader, and caller is not
		return nil, noAccessErr(ctx, prefix)
	}

	// We end up here if role is something other than READER, and the caller
	// doesn't have it. Maybe caller IS a reader, then we can give more concrete
	// error message.
	switch yes, err := hasRole(ctx, metas, repopb.Role_READER); {
	case err != nil:
		return nil, err
	case yes:
		return nil, status.Errorf(
			codes.PermissionDenied, "%q has no required %s role in prefix %q",
			auth.CurrentIdentity(ctx), role, prefix)
	default:
		return nil, noAccessErr(ctx, prefix)
	}
}

// noAccessErr produces a grpc error saying that the given prefix doesn't
// exist or the caller has no access to it. This is generic error message that
// should not give away prefix presence to non-readers.
func noAccessErr(ctx context.Context, prefix string) error {
	var msg string
	if ident := auth.CurrentIdentity(ctx); ident.Kind() == identity.Anonymous {
		msg = "not visible to unauthenticated callers"
	} else {
		msg = fmt.Sprintf("%q is not allowed to see it", ident)
	}
	return status.Errorf(codes.PermissionDenied, "prefix %q doesn't exist or %s", prefix, msg)
}

// noMetadataErr produces a grpc error saying that the given prefix doesn't have
// metadata attached.
func noMetadataErr(prefix string) error {
	return status.Errorf(codes.NotFound, "prefix %q has no metadata", prefix)
}

////////////////////////////////////////////////////////////////////////////////
// Prefix listing.

// ListPrefix implement the corresponding RPC method, see the proto doc.
func (impl *repoImpl) ListPrefix(ctx context.Context, r *repopb.ListPrefixRequest) (resp *repopb.ListPrefixResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	r.Prefix, err = common.ValidatePackagePrefix(r.Prefix)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'prefix': %s", err)
	}

	// Discover prefixes the caller is allowed to see. Note that checking only
	// r.Prefix ACL is not sufficient, since the caller may not see it, but still
	// see some deeper prefix, thus we need to enumerate the metadata subtree.
	var visibleRoots []string // sorted list of prefixes visible to the caller
	err = impl.meta.VisitMetadata(ctx, r.Prefix, func(pfx string, md []*repopb.PrefixMetadata) (cont bool, err error) {
		switch visible, err := hasRole(ctx, md, repopb.Role_READER); {
		case err != nil:
			return false, err
		case visible:
			// Found a visible root. Everything under 'pfx' is visible to the caller,
			// no sense in recursing deeper into this subtree.
			visibleRoots = append(visibleRoots, pfx)
			return false, nil
		default:
			// Continue exploring this subtree until we find something visible there,
			// if anything.
			return true, nil
		}
	})
	if err != nil {
		return nil, errors.Fmt("failed to enumerate the metadata: %w", err)
	}

	// One of 'visibleRoots' => its full sorted recursive listing.
	perVisibleRoot := make(map[string][]string, len(visibleRoots))

	// ListPackages used below lists only packages that are under the prefix. It
	// is possible there are packages that match some of visibleRoots directly
	// (have exact same name as some visible root). We check it here. Note that
	// this applies only to subprefixes of r.Prefix: we don't want to return a
	// package named r.Prefix in the result (if any). So if visibleRoots is
	// [r.Prefix] itself, we skip this check. Note that if r.Prefix is in
	// visibleRoots, then it is the only item there, by construction.
	if len(visibleRoots) != 1 || visibleRoots[0] != r.Prefix {
		rootPkgs, err := model.CheckPackages(ctx, visibleRoots, r.IncludeHidden)
		if err != nil {
			return nil, errors.Fmt("failed to check presence of packages: %w", err)
		}
		for _, name := range rootPkgs {
			perVisibleRoot[name] = []string{name}
		}
	}

	// Fetch the listing of each visible root in parallel. Note that there's no
	// intersection between them due to the order of VisitMetadata enumeration.
	err = parallel.WorkPool(8, func(tasks chan<- func() error) {
		mu := sync.Mutex{}
		for _, pfx := range visibleRoots {
			tasks <- func() error {
				// TODO(vadimsh): This is inefficient for non-recursive listings.
				// Unfortunately we have to do the recursive listing to discover
				// subprefixes. E.g. when listing 'a', we need to return 'a/b' as a
				// subprefix if there's a package 'a/b/c', so to discover 'a/b/c' we
				// need full recursive listing of 'a'.
				//
				// One possible improvement is:
				//  1. Introduce a new parallel entity group just for browsing:
				//     Entry {
				//       Name: <full path to the package or the prefix>,
				//       Children: [<names of all child prefixes and packages>],
				//       HiddenChildren: [<names of hidden prefixes and packages>],
				//       Hidden: <true|false>,  // also true if all children are hidden
				//     }
				//  2. 'Entry' basically represents a browsable item (either a package
				//     or a "directory" aka prefix).
				//  3. Update this entity group when registering, deleting, hiding or
				//     showing packages. Will require O(<number of path components>)
				//     entity writes in a single transaction, which is acceptable.
				//  4. In non-recursive listing just look at Children of corresponding
				//     entity.
				listing, err := model.ListPackages(ctx, pfx, r.IncludeHidden)
				if err == nil {
					mu.Lock()
					perVisibleRoot[pfx] = append(perVisibleRoot[pfx], listing...)
					mu.Unlock()
				}
				return err
			}
		}
	})
	if err != nil {
		return nil, errors.Fmt("failed to list a prefix: %w", err)
	}

	// searchRoot is lexicographical prefix of packages we are concerned about.
	searchRoot := r.Prefix
	if searchRoot != "" {
		searchRoot += "/"
	}

	// Visit all discovered packages (in sorted order).
	resp = &repopb.ListPrefixResponse{}
	dirs := stringset.New(0)
	for _, pfx := range visibleRoots {
		for _, pkg := range perVisibleRoot[pfx] {
			// By construction, ALL packages here must be within the search root.
			if !strings.HasPrefix(pkg, searchRoot) {
				panic(fmt.Sprintf(
					"unexpected package %q in results when listing %q, visible roots are %q",
					pkg, searchRoot, visibleRoots))
			}

			// E.g. "a/b/c", relative to searchRoot.
			rel := strings.TrimPrefix(pkg, searchRoot)

			if r.Recursive {
				// If listing recursively, add everything to the result.
				resp.Packages = append(resp.Packages, pkg)
				// For rel "a/b/c", add [".../a", ".../a/b"] to the directories set.
				if chunks := strings.Split(rel, "/"); len(chunks) > 1 {
					for i := 1; i < len(chunks); i++ {
						dirs.Add(searchRoot + strings.Join(chunks[:i], "/"))
					}
				}
			} else {
				// Otherwise add only packages that directly reside under searchRoot,
				// and pick only first path component of directories, since we don't
				// care about what's deeper.
				if idx := strings.IndexRune(rel, '/'); idx != -1 {
					// 'rel' has / => it is a directory, add its first path component.
					dirs.Add(searchRoot + rel[:idx])
				} else {
					// 'rel' has no / inside => it's a package directly under searchRoot.
					resp.Packages = append(resp.Packages, pkg)
				}
			}
		}
	}

	// Note: resp.Packages are already sorted by construction.
	resp.Prefixes = dirs.ToSlice()
	sort.Strings(resp.Prefixes)

	return resp, nil
}

////////////////////////////////////////////////////////////////////////////////
// Hide/unhide package.

// HidePackage implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) HidePackage(ctx context.Context, r *repopb.PackageRequest) (*emptypb.Empty, error) {
	return impl.setPackageHidden(ctx, r, model.Hidden)
}

// UnhidePackage implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) UnhidePackage(ctx context.Context, r *repopb.PackageRequest) (*emptypb.Empty, error) {
	return impl.setPackageHidden(ctx, r, model.Visible)
}

// setPackageHidden is common implementation of HidePackage and UnhidePackage.
func (impl *repoImpl) setPackageHidden(ctx context.Context, r *repopb.PackageRequest, hidden bool) (resp *emptypb.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_OWNER); err != nil {
		return nil, err
	}

	switch err := model.SetPackageHidden(ctx, r.Package, hidden); {
	case err == datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "no such package")
	case err != nil:
		return nil, errors.Fmt("failed to update the package: %w", err)
	}
	return &emptypb.Empty{}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Package deletion.

// DeletePackage implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) DeletePackage(ctx context.Context, r *repopb.PackageRequest) (resp *emptypb.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}

	// DeletePackage RPC, due to its potential severity, is limited only to
	// administrators, i.e. OWNERs of the repository root (prefix ""). Before
	// checking this, make sure the caller can otherwise modify the package at
	// all, to give a nicer error message if they can't.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_OWNER); err != nil {
		return nil, err
	}
	if _, err := impl.checkRole(ctx, "", repopb.Role_OWNER); err != nil {
		if status.Code(err) == codes.PermissionDenied {
			return nil, status.Errorf(codes.PermissionDenied,
				"package deletion is allowed only to service administrators")
		}
		return nil, err
	}

	return &emptypb.Empty{}, model.DeletePackage(ctx, r.Package)
}

////////////////////////////////////////////////////////////////////////////////
// Package instance registration and post-registration processing.

// RegisterInstance implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) RegisterInstance(ctx context.Context, r *repopb.Instance) (resp *repopb.RegisterInstanceResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request format.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if err := common.ValidateObjectRef(r.Instance, common.KnownHash); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'instance': %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_WRITER); err != nil {
		return nil, err
	}

	// Is such instance already registered?
	instance := (&model.Instance{}).FromProto(ctx, r)
	switch err := datastore.Get(ctx, instance); {
	case err == nil:
		return &repopb.RegisterInstanceResponse{
			Status:   repopb.RegistrationStatus_ALREADY_REGISTERED,
			Instance: instance.Proto(),
		}, nil
	case err != datastore.ErrNoSuchEntity:
		return nil, errors.Fmt("failed to fetch the instance entity: %w", err)
	}

	// Attempt to start a new upload session. This will fail with ALREADY_EXISTS
	// if such object is already in the storage. This is expected (it means the
	// client has uploaded the object already and we should just register the
	// instance right away).
	uploadOp, err := impl.cas.BeginUpload(ctx, &caspb.BeginUploadRequest{
		Object: r.Instance,
	})
	switch code := status.Code(err); {
	case code == codes.AlreadyExists:
		break // the object is already there
	case code == codes.OK:
		// The object is not in the storage and we have just started the upload. Let
		// the client finish it.
		return &repopb.RegisterInstanceResponse{
			Status:   repopb.RegistrationStatus_NOT_UPLOADED,
			UploadOp: uploadOp,
		}, nil
	default:
		return nil, errors.Fmt("failed to initiate an upload op (code %s): %w", code, err)
	}

	// Warn about registering deprecated SHA1 packages. Eventually this will be
	// forbidden completely.
	if r.Instance.HashAlgo == caspb.HashAlgo_SHA1 {
		logging.Warningf(ctx, "Deprecated SHA1 instance: %s (%s) from %s",
			r.Package, common.ObjectRefToInstanceID(r.Instance), auth.CurrentIdentity(ctx))
	}

	// The instance is already in the CAS storage. Register it in the repository.
	instance = (&model.Instance{
		RegisteredBy: string(auth.CurrentIdentity(ctx)),
		RegisteredTs: clock.Now(ctx).UTC(),
	}).FromProto(ctx, r)
	registered, instance, err := model.RegisterInstance(ctx, instance, impl.onInstanceRegistration)
	if err != nil {
		return nil, errors.Fmt("failed to register the instance: %w", err)
	}

	resp = &repopb.RegisterInstanceResponse{Instance: instance.Proto()}
	if registered {
		resp.Status = repopb.RegistrationStatus_REGISTERED
	} else {
		resp.Status = repopb.RegistrationStatus_ALREADY_REGISTERED
	}
	return
}

// onInstanceRegistration is called in a txn when registering an instance.
func (impl *repoImpl) onInstanceRegistration(ctx context.Context, inst *model.Instance) error {
	// Collect IDs of applicable processors.
	var procs []string
	for _, p := range impl.procs {
		switch yes, err := p.Applicable(ctx, inst); {
		case err != nil:
			return transient.Tag.Apply(errors.Fmt("failed to check applicability of processor %q: %w", p.ID(), err))
		case yes:
			procs = append(procs, p.ID())
		}
	}
	if len(procs) == 0 {
		return nil
	}

	// Mark the instance as being processed now.
	inst.ProcessorsPending = procs

	// Launch the TQ task that does the processing (see runProcessorsTask below).
	return impl.tq.AddTask(ctx, &tq.Task{
		Title:   inst.InstanceID,
		Payload: &tasks.RunProcessors{Instance: inst.Proto()},
	})
}

// runProcessorsTask executes a post-upload processing step.
//
// Returning a transient error here causes the task queue service to retry the
// task.
func (impl *repoImpl) runProcessorsTask(ctx context.Context, t *tasks.RunProcessors) error {
	// Fetch the instance to see what processors are still pending.
	inst := (&model.Instance{}).FromProto(ctx, t.Instance)
	switch err := datastore.Get(ctx, inst); {
	case err == datastore.ErrNoSuchEntity:
		return fmt.Errorf("instance %q is unexpectedly gone from the datastore", inst.InstanceID)
	case err != nil:
		return transient.Tag.Apply(err)
	}

	results := map[string]processing.Result{}

	// Grab processors we haven't ran yet.
	var run []processing.Processor
	for _, id := range inst.ProcessorsPending {
		if proc := impl.procsMap[id]; proc != nil {
			run = append(run, proc)
		} else {
			logging.Errorf(ctx, "Skipping unknown processor %q", id)
			results[id] = processing.Result{Err: fmt.Errorf("unknown processor %q", id)}
		}
	}

	// Exit early if there's nothing to run.
	if len(run) == 0 {
		return impl.updateProcessors(ctx, t.Instance, results)
	}

	// Open the package for reading.
	pkg, err := impl.packageReader(ctx, t.Instance.Instance)
	switch {
	case transient.Tag.In(err):
		return err // retry the whole thing
	case err != nil:
		// The package is fatally broken, give up.
		logging.WithError(err).Errorf(ctx, "The package can't be opened, failing all processors")
		for _, proc := range run {
			results[proc.ID()] = processing.Result{Err: err}
		}
		return impl.updateProcessors(ctx, t.Instance, results)
	}

	// Run the processors sequentially, since PackageReader is not very friendly
	// to concurrent access.
	var transientErrs errors.MultiError
	for _, proc := range run {
		logging.Infof(ctx, "Running processor %q", proc.ID())
		res, err := proc.Run(ctx, inst, pkg)
		if err != nil {
			logging.WithError(err).Errorf(ctx, "Processor %q failed transiently", proc.ID())
			transientErrs = append(transientErrs, err)
		} else {
			if res.Err != nil {
				logging.WithError(res.Err).Errorf(ctx, "Processor %q failed fatally", proc.ID())
			}
			results[proc.ID()] = res
		}
	}

	// Store what we've got, even if some processor may have failed to run.
	updErr := impl.updateProcessors(ctx, t.Instance, results)

	// Prefer errors from processors over 'updErr' if both happen. Processor
	// errors are more interesting.
	switch {
	case len(transientErrs) != 0:
		return transient.Tag.Apply(transientErrs)
	case updErr != nil:
		return updErr
	}
	return nil
}

// updateProcessors transactionally creates ProcessingResult entities and
// updates Instance.Processors* fields.
func (impl *repoImpl) updateProcessors(ctx context.Context, inst *repopb.Instance, results map[string]processing.Result) error {
	if len(results) == 0 {
		return nil
	}

	instEnt := (&model.Instance{}).FromProto(ctx, inst)
	instKey := datastore.KeyForObj(ctx, instEnt)

	now := clock.Now(ctx).UTC()

	// Create ProcessingResult outside the transaction, since this involves slow
	// zlib compression in WriteResult.
	procResults := make(map[string]*model.ProcessingResult, len(results))
	for procID, res := range results {
		procRes := &model.ProcessingResult{
			ProcID:    procID,
			Instance:  instKey,
			CreatedTs: now,
			Success:   res.Err == nil,
		}
		procResults[procID] = procRes

		// If the result is not serializable, store the serialization error instead.
		err := res.Err
		if err == nil {
			if err = procRes.WriteResult(res.Result); err != nil {
				err = errors.Fmt("failed to write the processing result: %w", err)
			}
		}
		if err != nil {
			procRes.Success = false
			procRes.Error = err.Error()
		}
	}

	// Mutate Instance entity, storing results that haven't been stored yet.
	return model.Txn(ctx, "updateProcessors", func(ctx context.Context) error {
		switch err := datastore.Get(ctx, instEnt); {
		case err == datastore.ErrNoSuchEntity:
			return fmt.Errorf("the entity is unexpectedly gone")
		case err != nil:
			return transient.Tag.Apply(errors.Fmt("failed to fetch the entity: %w", err))
		}

		var toPut []any

		// Go over what's is still pending, and move it to either Success or Failure
		// group if it is done.
		stillPending := instEnt.ProcessorsPending[:0]
		for _, procID := range instEnt.ProcessorsPending {
			res, done := procResults[procID]
			if !done {
				stillPending = append(stillPending, procID)
				continue
			}
			toPut = append(toPut, res)
			if res.Success {
				instEnt.ProcessorsSuccess = append(instEnt.ProcessorsSuccess, procID)
			} else {
				instEnt.ProcessorsFailure = append(instEnt.ProcessorsFailure, procID)
			}
		}
		instEnt.ProcessorsPending = stillPending

		// Store all the changes (if any).
		if len(toPut) == 0 {
			return nil
		}
		return transient.Tag.Apply(datastore.Put(ctx, toPut, instEnt))
	})
}

////////////////////////////////////////////////////////////////////////////////
// Instance listing and querying.

type paginatedQueryOpts struct {
	Package   string
	PageToken string
	PageSize  int32
	Validator func() error // validates callback-specific fields in the request
	Handler   func(cur datastore.Cursor, pageSize int32) ([]*model.Instance, datastore.Cursor, error)
}

// paginatedQuery is a common part of ListInstances and SearchInstances.
func (impl *repoImpl) paginatedQuery(ctx context.Context, opts paginatedQueryOpts) (out []*repopb.Instance, nextTok string, err error) {
	// Validate the request, decode the cursor.
	if err := common.ValidatePackageName(opts.Package); err != nil {
		return nil, "", status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}

	switch {
	case opts.PageSize < 0:
		return nil, "", status.Errorf(codes.InvalidArgument, "bad 'page_size' %d: it should be non-negative", opts.PageSize)
	case opts.PageSize == 0:
		opts.PageSize = 100
	}

	var cursor datastore.Cursor
	if opts.PageToken != "" {
		if cursor, err = datastore.DecodeCursor(ctx, opts.PageToken); err != nil {
			return nil, "", status.Errorf(codes.InvalidArgument, "bad 'page_token': %s", err)
		}
	}

	if opts.Validator != nil {
		if err := opts.Validator(); err != nil {
			return nil, "", err
		}
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, opts.Package, repopb.Role_READER); err != nil {
		return nil, "", err
	}

	// Check that the package is registered.
	if err := model.CheckPackageExists(ctx, opts.Package); err != nil {
		return nil, "", err
	}

	// Do the actual listing.
	inst, cursor, err := opts.Handler(cursor, opts.PageSize)
	if err != nil {
		return nil, "", errors.Fmt("failed to query instances: %w", err)
	}

	// Convert results to proto.
	out = make([]*repopb.Instance, len(inst))
	for i, ent := range inst {
		out[i] = ent.Proto()
	}
	if cursor != nil {
		nextTok = cursor.String()
	}
	return
}

// ListInstances implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) ListInstances(ctx context.Context, r *repopb.ListInstancesRequest) (resp *repopb.ListInstancesResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	result, nextPage, err := impl.paginatedQuery(ctx, paginatedQueryOpts{
		Package:   r.Package,
		PageSize:  r.PageSize,
		PageToken: r.PageToken,
		Handler: func(cur datastore.Cursor, pageSize int32) ([]*model.Instance, datastore.Cursor, error) {
			return model.ListInstances(ctx, r.Package, pageSize, cur)
		},
	})
	if err != nil {
		return nil, err
	}

	return &repopb.ListInstancesResponse{
		Instances:     result,
		NextPageToken: nextPage,
	}, nil
}

// SearchInstances implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) SearchInstances(ctx context.Context, r *repopb.SearchInstancesRequest) (resp *repopb.SearchInstancesResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	who := auth.CurrentIdentity(ctx)
	for _, t := range r.Tags {
		logging.Infof(ctx, "SearchInstances: %s %s %s:%s", who, r.Package, t.Key, t.Value)
	}

	result, nextPage, err := impl.paginatedQuery(ctx, paginatedQueryOpts{
		Package:   r.Package,
		PageSize:  r.PageSize,
		PageToken: r.PageToken,
		Validator: func() error { return validateTagList(r.Tags) },
		Handler: func(cur datastore.Cursor, pageSize int32) ([]*model.Instance, datastore.Cursor, error) {
			return model.SearchInstances(ctx, r.Package, r.Tags, pageSize, cur)
		},
	})
	if err != nil {
		return nil, err
	}

	return &repopb.SearchInstancesResponse{
		Instances:     result,
		NextPageToken: nextPage,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Refs support.

// CreateRef implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) CreateRef(ctx context.Context, r *repopb.Ref) (resp *emptypb.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageRef(r.Name); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'name': %s", err)
	}
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if err := common.ValidateObjectRef(r.Instance, common.KnownHash); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'instance': %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_WRITER); err != nil {
		return nil, err
	}

	// Actually create or move the ref. This will also transactionally check the
	// instance exists and it has passed the processing successfully.
	inst := &model.Instance{
		InstanceID: common.ObjectRefToInstanceID(r.Instance),
		Package:    model.PackageKey(ctx, r.Package),
	}
	if err := model.SetRef(ctx, r.Name, inst); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DeleteRef implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) DeleteRef(ctx context.Context, r *repopb.DeleteRefRequest) (resp *emptypb.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageRef(r.Name); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'name': %s", err)
	}
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_WRITER); err != nil {
		return nil, err
	}

	// Verify the package actually exists, per DeleteRef contract.
	if err := model.CheckPackageExists(ctx, r.Package); err != nil {
		return nil, err
	}

	// Actually delete the ref.
	if err := model.DeleteRef(ctx, r.Package, r.Name); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ListRefs implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) ListRefs(ctx context.Context, r *repopb.ListRefsRequest) (resp *repopb.ListRefsResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_READER); err != nil {
		return nil, err
	}

	// Verify the package actually exists, per ListPackageRefs contract.
	if err := model.CheckPackageExists(ctx, r.Package); err != nil {
		return nil, err
	}

	// Actually list refs.
	refs, err := model.ListPackageRefs(ctx, r.Package)
	if err != nil {
		return nil, errors.Fmt("failed to list refs: %w", err)
	}
	resp = &repopb.ListRefsResponse{Refs: make([]*repopb.Ref, len(refs))}
	for i, ref := range refs {
		resp.Refs[i] = ref.Proto()
	}
	return resp, nil
}

////////////////////////////////////////////////////////////////////////////////
// Tags support.

func validateTagList(tags []*repopb.Tag) error {
	if len(tags) == 0 {
		return status.Errorf(codes.InvalidArgument, "bad 'tags': cannot be empty")
	}
	for _, t := range tags {
		kv := common.JoinInstanceTag(t)
		if err := common.ValidateInstanceTag(kv); err != nil {
			return status.Errorf(codes.InvalidArgument, "bad tag in 'tags': %s", err)
		}
	}
	return nil
}

func validateMultiTagReq(pkg string, inst *caspb.ObjectRef, tags []*repopb.Tag) error {
	if err := common.ValidatePackageName(pkg); err != nil {
		return status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if err := common.ValidateObjectRef(inst, common.KnownHash); err != nil {
		return status.Errorf(codes.InvalidArgument, "bad 'instance': %s", err)
	}
	return validateTagList(tags)
}

// AttachTags implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) AttachTags(ctx context.Context, r *repopb.AttachTagsRequest) (resp *emptypb.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := validateMultiTagReq(r.Package, r.Instance, r.Tags); err != nil {
		return nil, err
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_WRITER); err != nil {
		return nil, err
	}

	// Actually attach the tags. This will also transactionally check the instance
	// exists and it has passed the processing successfully.
	inst := &model.Instance{
		InstanceID: common.ObjectRefToInstanceID(r.Instance),
		Package:    model.PackageKey(ctx, r.Package),
	}
	if err := model.AttachTags(ctx, inst, r.Tags); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DetachTags implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) DetachTags(ctx context.Context, r *repopb.DetachTagsRequest) (resp *emptypb.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := validateMultiTagReq(r.Package, r.Instance, r.Tags); err != nil {
		return nil, err
	}

	// Check ACLs. Note this is scoped to OWNERS, see the proto doc.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_OWNER); err != nil {
		return nil, err
	}

	// Verify the instance exists, per DetachTags contract.
	inst := &model.Instance{
		InstanceID: common.ObjectRefToInstanceID(r.Instance),
		Package:    model.PackageKey(ctx, r.Package),
	}
	if err := model.CheckInstanceExists(ctx, inst); err != nil {
		return nil, err
	}

	// Actually detach the tags.
	if err := model.DetachTags(ctx, inst, r.Tags); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Instance metadata support.

const (
	// vsaAttestationsKey is the key for metadata entries that contains
	// attestations.
	vsaAttestationsKey = "policy-attestations"
	// vsaAttestationsContentType is the content type for attestation bundle.
	vsaAttestationsContentType = "application/vnd.in-toto.bundle"
	// slsaVSAKey is the key for metadata entries that contains SLSA Verification
	// Summary Attestation.
	slsaVSAKey = "luci-slsa-vsa"
)

// AttachMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) AttachMetadata(ctx context.Context, r *repopb.AttachMetadataRequest) (resp *emptypb.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if err := common.ValidateObjectRef(r.Instance, common.KnownHash); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'instance': %s", err)
	}
	if len(r.Metadata) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'metadata': cannot be empty")
	}
	for _, m := range r.Metadata {
		if err := common.ValidateInstanceMetadataKey(m.Key); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad 'metadata' key: %s", err)
		}
		if strings.HasPrefix(m.Key, "luci-") {
			return nil, status.Errorf(codes.InvalidArgument, "bad 'metadata' key %q: `luci-` prefix is reserved for internal use", m.Key)
		}
		if err := common.ValidateInstanceMetadataLen(len(m.Value)); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "metadata with key %q: %s", m.Key, err)
		}
		if err := common.ValidateContentType(m.ContentType); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "metadata with key %q: %s", m.Key, err)
		}
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_WRITER); err != nil {
		return nil, err
	}

	inst := &model.Instance{
		InstanceID: common.ObjectRefToInstanceID(r.Instance),
		Package:    model.PackageKey(ctx, r.Package),
	}

	// Early check the instance ready to prevent retrying on
	// VerifySoftwareArtifact. When metadata is actually attached this will be
	// checked again in transaction to guarantee the instance's readiness.
	if err := model.CheckInstanceReady(ctx, inst); err != nil {
		return nil, err
	}

	// Call VerifySoftwareArtifact if an attestation bundle is attached to the package.
	var extraMetadata []*repopb.InstanceMetadata
	for _, m := range r.Metadata {
		if m.Key == vsaAttestationsKey {
			if vsa := impl.vsa.VerifySoftwareArtifact(ctx, inst, string(m.Value)); vsa != "" {
				extraMetadata = append(extraMetadata, &repopb.InstanceMetadata{
					Key:         slsaVSAKey,
					Value:       []byte(vsa),
					ContentType: vsaAttestationsContentType,
				})
			}
		}
	}
	r.Metadata = append(r.Metadata, extraMetadata...)

	// Actually attach the metadata. This will also transactionally check the
	// instance exists and it has passed the processing successfully.
	if err := model.AttachMetadata(ctx, inst, r.Metadata); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DetachMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) DetachMetadata(ctx context.Context, r *repopb.DetachMetadataRequest) (resp *emptypb.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if err := common.ValidateObjectRef(r.Instance, common.KnownHash); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'instance': %s", err)
	}
	if len(r.Metadata) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'metadata': cannot be empty")
	}
	for _, m := range r.Metadata {
		// If have a fingerprint, ignore Key and Value. Otherwise we need them
		// to calculate the fingerprint on the fly in model.DetachMetadata.
		if m.Fingerprint != "" {
			if err := common.ValidateInstanceMetadataFingerprint(m.Fingerprint); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "bad metadata: %s", err)
			}
		} else {
			if err := common.ValidateInstanceMetadataKey(m.Key); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "bad metadata key: %s", err)
			}
			if err := common.ValidateInstanceMetadataLen(len(m.Value)); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "metadata with key %q: %s", m.Key, err)
			}
		}
	}

	// Check ACLs. Require OWNER role for destructive operations.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_OWNER); err != nil {
		return nil, err
	}

	// Verify the instance exists to return more correct error message.
	inst := &model.Instance{
		InstanceID: common.ObjectRefToInstanceID(r.Instance),
		Package:    model.PackageKey(ctx, r.Package),
	}
	if err := model.CheckInstanceExists(ctx, inst); err != nil {
		return nil, err
	}

	// Actually detach the metadata.
	if err := model.DetachMetadata(ctx, inst, r.Metadata); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func listMetadata(ctx context.Context, inst *model.Instance, keys ...string) ([]*model.InstanceMetadata, error) {
	if len(keys) != 0 {
		return model.ListMetadataWithKeys(ctx, inst, keys)
	}

	return model.ListMetadata(ctx, inst)
}

// ListMetadata implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) ListMetadata(ctx context.Context, r *repopb.ListMetadataRequest) (resp *repopb.ListMetadataResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if err := common.ValidateObjectRef(r.Instance, common.KnownHash); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'instance': %s", err)
	}
	for _, k := range r.Keys {
		if err := common.ValidateInstanceMetadataKey(k); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad metadata key: %s", err)
		}
	}
	if r.PageToken != "" {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'page_token': not supported yet")
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_READER); err != nil {
		return nil, err
	}

	// Verify the instance exists to return more correct error message.
	inst := &model.Instance{
		InstanceID: common.ObjectRefToInstanceID(r.Instance),
		Package:    model.PackageKey(ctx, r.Package),
	}
	if err := model.CheckInstanceExists(ctx, inst); err != nil {
		return nil, err
	}

	// Actually list metadata.
	var md []*model.InstanceMetadata
	md, err = listMetadata(ctx, inst, r.Keys...)
	if err != nil {
		return nil, err
	}
	resp = &repopb.ListMetadataResponse{
		Metadata: make([]*repopb.InstanceMetadata, len(md)),
	}
	for i, m := range md {
		resp.Metadata[i] = m.Proto()
	}
	return resp, nil
}

// ensureVSA check whether the instance has a vsa. If not, it will create a
// asynchronously task using VerifySoftwareArtifact to attach the vsa.
func (impl *repoImpl) ensureVSA(ctx context.Context, inst *model.Instance) error {
	switch s, err := impl.vsa.GetStatus(ctx, inst); {
	case err != nil:
		return err
	case s != vsa.CacheStatusUnknown:
		// We already have a vsa or we have a pending vsa request.
		return nil
	}
	if err := impl.vsa.SetStatus(ctx, inst, vsa.CacheStatusPending); err != nil {
		return err
	}

	vsas, err := model.ListMetadataWithKeys(ctx, inst, []string{slsaVSAKey})
	if err != nil {
		return err
	}

	if len(vsas) != 0 {
		return impl.vsa.SetStatus(ctx, inst, vsa.CacheStatusCompleted)
	}

	ms, err := model.ListMetadataWithKeys(ctx, inst, []string{vsaAttestationsKey})
	if err != nil {
		return err
	}

	var bundle string
	if len(ms) > 0 {
		bundle = string(ms[0].Value)
	}

	t := impl.vsa.NewVerifySoftwareArtifactTask(ctx, inst, bundle)
	return impl.tq.AddTask(ctx, &tq.Task{
		Title:            inst.InstanceID,
		Payload:          t,
		DeduplicationKey: inst.InstanceID,
	})
}

func (impl *repoImpl) callVerifySoftwareArtifact(ctx context.Context, t *tasks.CallVerifySoftwareArtifact) error {
	vsaStr := impl.vsa.CallVerifySoftwareArtifact(ctx, t)
	if vsaStr == "" {
		return nil
	}

	inst := (&model.Instance{}).FromProto(ctx, t.Instance)
	if err := model.AttachMetadata(ctx, inst, []*repopb.InstanceMetadata{{
		Key:         slsaVSAKey,
		Value:       []byte(vsaStr),
		ContentType: vsaAttestationsContentType,
	}}); err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to attach VSA metadata to %s", inst.InstanceID)
		return nil // log and ignore
	}

	if err := impl.vsa.SetStatus(ctx, inst, vsa.CacheStatusCompleted); err != nil {
		logging.WithError(err).Warningf(ctx, "Cache VSA Status: %s", inst.Package)
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Version resolution and instance info fetching.

// ResolveVersion implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) ResolveVersion(ctx context.Context, r *repopb.ResolveVersionRequest) (resp *repopb.Instance, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if err := common.ValidateInstanceVersion(r.Version); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'version': %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_READER); err != nil {
		return nil, err
	}

	// Actually resolve the version. This will return an appropriately grpc-tagged
	// error.
	inst, err := model.ResolveVersion(ctx, r.Package, r.Version)
	if err != nil {
		return nil, err
	}
	return inst.Proto(), nil
}

// GetInstanceURL implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) GetInstanceURL(ctx context.Context, r *repopb.GetInstanceURLRequest) (resp *caspb.ObjectURL, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if err := common.ValidateObjectRef(r.Instance, common.KnownHash); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'instance': %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_READER); err != nil {
		return nil, err
	}

	// Make sure this instance actually exists (without this check the caller
	// would be able to "probe" CAS namespace unrestricted).
	inst := (&model.Instance{}).FromProto(ctx, &repopb.Instance{
		Package:  r.Package,
		Instance: r.Instance,
	})
	if err := model.CheckInstanceExists(ctx, inst); err != nil {
		return nil, err
	}

	if err := impl.ensureVSA(ctx, inst); err != nil {
		logging.WithError(err).Warningf(ctx, "ensuring VSA: %s", inst.Package)
	}

	// Ask CAS generate an URL for us. Note that CAS does caching internally.
	return impl.cas.GetObjectURL(ctx, &caspb.GetObjectURLRequest{
		Object: r.Instance,
	})
}

// DescribeInstance implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) DescribeInstance(ctx context.Context, r *repopb.DescribeInstanceRequest) (resp *repopb.DescribeInstanceResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if err := common.ValidateObjectRef(r.Instance, common.KnownHash); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'instance': %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_READER); err != nil {
		return nil, err
	}

	// Make sure this instance exists and fetch basic details about it.
	inst := (&model.Instance{}).FromProto(ctx, &repopb.Instance{
		Package:  r.Package,
		Instance: r.Instance,
	})
	if err := model.CheckInstanceExists(ctx, inst); err != nil {
		return nil, err
	}

	// Fetch the rest based on what the client wants.
	var refs []*model.Ref
	var tags []*model.Tag
	var proc []*repopb.Processor
	var mds []*model.InstanceMetadata
	err = parallel.FanOutIn(func(tasks chan<- func() error) {
		if r.DescribeRefs {
			tasks <- func() error {
				var err error
				refs, err = model.ListInstanceRefs(ctx, inst)
				return errors.WrapIf(err, "failed to fetch refs")
			}
		}
		if r.DescribeTags {
			tasks <- func() error {
				var err error
				tags, err = model.ListInstanceTags(ctx, inst)
				return errors.WrapIf(err, "failed to fetch tags")
			}
		}
		if r.DescribeProcessors {
			tasks <- func() error {
				var err error
				proc, err = model.FetchProcessors(ctx, inst)
				return errors.WrapIf(err, "failed to fetch processors")
			}
		}
		if r.DescribeMetadata {
			tasks <- func() error {
				var err error
				mds, err = listMetadata(ctx, inst)
				return errors.WrapIf(err, "failed to fetch metadata")
			}
		}
	})
	if err != nil {
		return nil, err // note: this is always Internal error
	}

	// Assemble the resulting proto. Apparently nil and empty slices are treated
	// differently by jsonpb, so make sure to keep empty fields as nils.
	resp = &repopb.DescribeInstanceResponse{Instance: inst.Proto()}
	if len(refs) != 0 {
		resp.Refs = make([]*repopb.Ref, len(refs))
		for i, r := range refs {
			resp.Refs[i] = r.Proto()
		}
	}
	if len(tags) != 0 {
		resp.Tags = make([]*repopb.Tag, len(tags))
		for i, t := range tags {
			resp.Tags[i] = t.Proto()
		}
	}
	if len(proc) != 0 {
		resp.Processors = proc
	}
	if len(mds) != 0 {
		resp.Metadata = make([]*repopb.InstanceMetadata, len(mds))
		for i, md := range mds {
			resp.Metadata[i] = md.Proto()
		}
	}
	return resp, nil
}

// DescribeClient implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) DescribeClient(ctx context.Context, r *repopb.DescribeClientRequest) (resp *repopb.DescribeClientResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': %s", err)
	}
	if !processing.IsClientPackage(r.Package) {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package': not a CIPD client package")
	}
	if err := common.ValidateObjectRef(r.Instance, common.KnownHash); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'instance': %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Package, repopb.Role_READER); err != nil {
		return nil, err
	}

	// Make sure this instance exists, has all processors finished and fetch
	// basic details about it.
	inst := (&model.Instance{}).FromProto(ctx, &repopb.Instance{
		Package:  r.Package,
		Instance: r.Instance,
	})
	if err := model.CheckInstanceReady(ctx, inst); err != nil {
		return nil, err
	}

	// Grab the location of the extracted CIPD client from the post-processor.
	// This must succeed, since CheckInstanceReady above verified processors have
	// finished. Thus treat any error here as internal, as it will require an
	// investigation.
	proc, err := processing.GetClientExtractorResult(ctx, inst.Proto())
	if err != nil {
		return nil, grpcutil.InternalTag.Apply(errors.Fmt("failed to get client extractor results: %w", err))
	}
	ref, err := proc.ToObjectRef()
	if err != nil {
		return nil, grpcutil.InternalTag.Apply(errors.

			// refAliases (and SHA1 in particular, as hash supported by oldest code) is
			// required to allow older clients to self-update to a newer client. See the
			// doc for DescribeClientResponse proto message.
			Fmt("malformed or unrecognized ref in the client extractor results: %w", err))
	}

	refAliases := proc.ObjectRefAliases()
	sha1 := ""
	for _, ref := range refAliases {
		if ref.HashAlgo == caspb.HashAlgo_SHA1 {
			sha1 = ref.HexDigest
			break
		}
	}
	if sha1 == "" {
		return nil, grpcutil.InternalTag.Apply(errors.

			// Grab the signed URL of the client binary.
			New("malformed client extraction results, missing SHA1 digest"))
	}

	signedURL, err := impl.cas.GetObjectURL(ctx, &caspb.GetObjectURLRequest{
		Object:           ref,
		DownloadFilename: processing.GetClientBinaryName(r.Package), // e.g. 'cipd.exe'
	})
	if err != nil {
		return nil, grpcutil.InternalTag.Apply(errors.Fmt("failed to get signed URL to the client binary: %w", err))
	}

	return &repopb.DescribeClientResponse{
		Instance:         inst.Proto(),
		ClientRef:        ref,
		ClientBinary:     signedURL,
		ClientSize:       proc.ClientBinary.Size,
		LegacySha1:       sha1,
		ClientRefAliases: refAliases,
	}, nil
}

func (impl *repoImpl) DescribeBootstrapBundle(ctx context.Context, r *repopb.DescribeBootstrapBundleRequest) (resp *repopb.DescribeBootstrapBundleResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	// Validate the request.
	r.Prefix, err = common.ValidatePackagePrefix(r.Prefix)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'prefix': %s", err)
	}
	seen := stringset.New(len(r.Variants))
	for _, v := range r.Variants {
		if strings.Contains(v, "/") {
			return nil, status.Errorf(codes.InvalidArgument, "bad 'variant' %q: contains \"/\"", v)
		}
		if err := common.ValidatePackageName(r.Prefix + "/" + v); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad 'variant': %s", err)
		}
		if !seen.Add(v) {
			return nil, status.Errorf(codes.InvalidArgument, "variant %q was given more than once", v)
		}
	}
	if err := common.ValidateInstanceVersion(r.Version); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'version': %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(ctx, r.Prefix, repopb.Role_READER); err != nil {
		return nil, err
	}

	// Auto-discover all available packages if necessary.
	if len(r.Variants) == 0 {
		// Note: ListPackages does recursive prefix listing.
		listing, err := model.ListPackages(ctx, r.Prefix, false)
		if err != nil {
			return nil, err
		}
		for _, pkg := range listing {
			if !strings.HasPrefix(pkg, r.Prefix+"/") {
				panic(fmt.Sprintf("ListPackages %q unexpected returned package %q", r.Prefix, pkg))
			}
			// `pkg` is either <prefix>/<variant> or <prefix>/<some>/<subpackage>.
			// We are interested in the first case only.
			if pkg = strings.TrimPrefix(pkg, r.Prefix+"/"); !strings.Contains(pkg, "/") {
				r.Variants = append(r.Variants, pkg)
			}
		}
		if len(r.Variants) == 0 {
			return nil, status.Errorf(codes.NotFound, "no packages directly under prefix %q", r.Prefix)
		}
	}

	mu := sync.Mutex{}
	files := make(map[string]*repopb.DescribeBootstrapBundleResponse_BootstrapFile, len(r.Variants))

	if len(r.Variants) == 1 {
		// A resolution of only one variant is a very common case (in particular it
		// is used via handleBootstrapDownload). Avoid a work pool overhead.
		files[r.Variants[0]], err = impl.describeBootstrapFile(ctx, r.Prefix+"/"+r.Variants[0], r.Version)
	} else {
		// Call describeBootstrapFile in parallel for each requested variant.
		err = parallel.WorkPool(16, func(tasks chan<- func() error) {
			for _, variant := range r.Variants {
				tasks <- func() error {
					bf, err := impl.describeBootstrapFile(ctx, r.Prefix+"/"+variant, r.Version)
					mu.Lock()
					files[variant] = bf
					mu.Unlock()
					return err
				}
			}
		})
	}
	if err != nil {
		// describeBootstrapFile returns only transient errors as `err`. Fatal
		// error are returned via `status` field in BootstrapFile and handled below.
		return nil, transient.Tag.Apply(err)
	}

	// Order the output based on r.Variants.
	resp = &repopb.DescribeBootstrapBundleResponse{
		Files: make([]*repopb.DescribeBootstrapBundleResponse_BootstrapFile, 0, len(files)),
	}
	for _, v := range r.Variants {
		resp.Files = append(resp.Files, files[v])
	}

	// If *all* requests fatally failed with the same status code, use it as
	// the overall status. Otherwise return OK status, with errors communicated
	// via `status` fields in the response body.
	curCode := resp.Files[0].Status.GetCode()
	allAgree := true
	for i := 1; i < len(resp.Files); i++ {
		if resp.Files[i].Status.GetCode() != curCode {
			allAgree = false
			break
		}
	}
	if allAgree && curCode != int32(codes.OK) {
		extra := ""
		if len(resp.Files) > 1 {
			extra = fmt.Sprintf(" (and %d other errors like this)", len(resp.Files)-1)
		}
		return nil, status.Errorf(codes.Code(curCode), "%s%s", resp.Files[0].Status.GetMessage(), extra)
	}

	return
}

// describeBootstrapFile describes one extracted bootstrap file.
//
// Returns only transient errors as `err`. Fatal errors are communicated via
// `status` field in the response.
func (impl *repoImpl) describeBootstrapFile(ctx context.Context, pkg, ver string) (resp *repopb.DescribeBootstrapBundleResponse_BootstrapFile, err error) {
	defer func() {
		err = grpcutil.GRPCifyAndLogErr(ctx, err)
		if err != nil && !grpcutil.IsTransientCode(status.Code(err)) {
			resp = &repopb.DescribeBootstrapBundleResponse_BootstrapFile{
				Package: pkg,
				Status:  status.Convert(err).Proto(),
			}
			err = nil
		}
	}()

	// Resolve the version and verify the instance exists and finished processing.
	inst, err := model.ResolveVersion(ctx, pkg, ver)
	if err != nil {
		return nil, err
	}
	if err = inst.CheckReady(); err != nil {
		return nil, err
	}

	// Fetch the result of the bootstrap file extractor, if any.
	res, err := processing.GetBootstrapExtractorResult(ctx, inst)
	switch {
	case err == datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.FailedPrecondition, "%q is not a bootstrap package", pkg)
	case transient.Tag.In(err):
		return nil, err
	case err != nil:
		return nil, status.Errorf(codes.Aborted, "%q has broken bootstrap info: %s", pkg, err)
	}

	// Make sure the hash algo and hex digest are recognizable.
	fileRef := &caspb.ObjectRef{
		HashAlgo:  caspb.HashAlgo(caspb.HashAlgo_value[res.HashAlgo]),
		HexDigest: res.HashDigest,
	}
	if err = common.ValidateObjectRef(fileRef, common.AnyHash); err != nil {
		return nil, status.Errorf(codes.Aborted, "%q has broken bootstrap info: %s", pkg, err)
	}

	// Everything looks good.
	return &repopb.DescribeBootstrapBundleResponse_BootstrapFile{
		Package:  pkg,
		Instance: common.InstanceIDToObjectRef(inst.InstanceID),
		File:     fileRef,
		Name:     res.File,
		Size:     res.Size,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Non-pRPC handlers for the client bootstrap and legacy API.

const (
	// Name of a header that contains resolved CIPD instance IDs in /client, /dl
	// and /bootstrap responses.
	cipdInstanceHeader = "X-Cipd-Instance"
	// Header that contains the file digest for /bootstrap responses.
	cipdDigestHeader = "X-Cipd-File-Digest"
)

// legacyInstance is JSON representation of Instance in the legacy API.
type legacyInstance struct {
	PackageName  string `json:"package_name,omitempty"`
	InstanceID   string `json:"instance_id,omitempty"`
	RegisteredBy string `json:"registered_by,omitempty"`
	RegisteredTs string `json:"registered_ts,omitempty"` // timestamp in microsec
}

// FromInstance fills in legacyInstance based on data from Instance proto.
func (l *legacyInstance) FromInstance(inst *repopb.Instance) *legacyInstance {
	l.PackageName = inst.Package
	l.InstanceID = common.ObjectRefToInstanceID(inst.Instance)
	l.RegisteredBy = inst.RegisteredBy
	if ts := inst.RegisteredTs; ts != nil {
		l.RegisteredTs = fmt.Sprintf("%d", ts.Seconds*1e6+int64(ts.Nanos/1e3))
	} else {
		l.RegisteredTs = ""
	}
	return l
}

// adaptGrpcErr knows how to convert gRPC-style errors to ugly looking HTTP
// error pages with appropriate HTTP status codes.
//
// Recognizes either real gRPC errors (produced with status.Errorf) or
// grpc-tagged errors produced via grpcutil.
func adaptGrpcErr(h func(*router.Context) error) router.Handler {
	return func(ctx *router.Context) {
		err := status.Convert(grpcutil.GRPCifyAndLogErr(ctx.Request.Context(), h(ctx)))
		if err.Code() != codes.OK {
			http.Error(ctx.Writer, err.Message(), grpcutil.CodeStatus(err.Code()))
		}
	}
}

// parseDownloadPath splits "<package>/+/<version>" into components.
//
// Returns InvalidArgument gRPC errors.
func parseDownloadPath(path string) (pkg, ver string, err error) {
	chunks := strings.SplitN(strings.TrimPrefix(path, "/"), "/+/", 2)
	if len(chunks) != 2 {
		return "", "", status.Error(codes.InvalidArgument, "the URL should have form /.../<package>/+/<version>")
	}
	pkg, ver = chunks[0], chunks[1]
	if err = common.ValidatePackageName(pkg); err != nil {
		return "", "", status.Error(codes.InvalidArgument, err.Error())
	}
	if err = common.ValidateInstanceVersion(ver); err != nil {
		return "", "", status.Error(codes.InvalidArgument, err.Error())
	}
	return
}

// replyWithJSON sends StatusOK with JSON body.
func replyWithJSON(w http.ResponseWriter, obj any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(obj)
}

// replyWithError sends StatusOK with JSON body containing an error.
//
// Due to Cloud Endpoints limitations, legacy API used StatusOK for some not-OK
// responses and communicated the actual error through 'status' response field.
func replyWithError(w http.ResponseWriter, status, message string, args ...any) error {
	return replyWithJSON(w, map[string]string{
		"status":        status,
		"error_message": fmt.Sprintf(message, args...),
	})
}

// InstallHandlers installs non-pRPC HTTP handlers into the router.
//
// 'base' middleware chain here is assumed to have an authentication middleware
// that checks 'Authorization' header (not cookies!).
func (impl *repoImpl) InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.GET("/client", base, adaptGrpcErr(impl.handleClientBootstrap))
	r.GET("/dl/*path", base, adaptGrpcErr(impl.handlePackageDownload))
	r.GET("/bootstrap/*path", base, adaptGrpcErr(impl.handleBootstrapDownload))

	r.GET("/_ah/api/repo/v1/client", base, adaptGrpcErr(impl.handleLegacyClientInfo))
	r.GET("/_ah/api/repo/v1/instance", base, adaptGrpcErr(impl.handleLegacyInstance))
	r.GET("/_ah/api/repo/v1/instance/resolve", base, adaptGrpcErr(impl.handleLegacyResolve))

	// All other legacy endpoints (/_ah/api/repo/v1/*) just return an error asking
	// the client to update.
	r.NotFound(base, func(ctx *router.Context) {
		if strings.HasPrefix(ctx.Request.URL.Path, "/_ah/api/repo/v1/") {
			// Note: can't use http.Error here because it appends '\n' that renders
			// ugly by the client.
			ctx.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			ctx.Writer.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintf(ctx.Writer, "This version of CIPD client is no longer supported, please upgrade")
		} else {
			http.Error(ctx.Writer, "No such page", http.StatusNotFound)
		}
	})
}

// handleClientBootstrap redirects to a CIPD client binary in Google Storage.
//
// GET /client?platform=...&version=...
//
// Where:
//
//	platform: linux-amd64, windows-386, etc.
//	version: a package version identifier (instance ID, a ref or a tag).
//
// On success issues HTTP 302 redirect to the signed Google Storage URL.
// On errors returns HTTP 4** with an error message.
func (impl *repoImpl) handleClientBootstrap(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	// Do light validation (the rest is in ResolveVersion), and get a full client
	// package name.
	platform := r.FormValue("platform")
	version := r.FormValue("version")
	switch {
	case platform == "":
		return status.Errorf(codes.InvalidArgument, "no 'platform' specified")
	case version == "":
		return status.Errorf(codes.InvalidArgument, "no 'version' specified")
	}
	pkg, err := processing.GetClientPackage(platform)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "bad platform name")
	}

	// Resolve the version into a concrete instance. This also does rigorous
	// argument validation, ACL checks and verifies the resulting instance exists.
	inst, err := impl.ResolveVersion(c, &repopb.ResolveVersionRequest{
		Package: pkg,
		Version: version,
	})
	if err != nil {
		return err
	}

	// Put resolved instance ID into the response headers. This may be useful when
	// debugging fetches.
	w.Header().Set(cipdInstanceHeader, common.ObjectRefToInstanceID(inst.Instance))

	// Grab the location of the extracted CIPD client from the post-processor.
	res, err := processing.GetClientExtractorResult(c, inst)
	switch {
	case transient.Tag.In(err):
		return err
	case err == datastore.ErrNoSuchEntity:
		return status.Errorf(codes.NotFound, "the client binary is not extracted yet, try later")
	case err != nil: // fatal
		return status.Errorf(codes.NotFound, "the client binary is not available: %s", err)
	}
	ref, err := res.ToObjectRef()
	if err != nil {
		return status.Errorf(codes.NotFound, "malformed ref to the client binary: %s", err)
	}

	// Ask CAS for a signed URL to the client binary and redirect there.
	url, err := impl.cas.GetObjectURL(c, &caspb.GetObjectURLRequest{
		Object:           ref,
		DownloadFilename: processing.GetClientBinaryName(pkg), // e.g. 'cipd.exe'
	})
	if err == nil {
		http.Redirect(w, r, url.SignedUrl, http.StatusFound)
	}
	return err
}

// handlePackageDownload redirects to a CIPD package file (raw octet stream
// with zipped package data) in Google Storage.
//
// GET /dl/<package>/+/<version>.
//
// Where:
//
//	package: a CIPD package name (e.g. "a/b/c/linux-amd64").
//	version: a package version identifier (instance ID, a ref or a tag).
//
// On success issues HTTP 302 redirect to the signed Google Storage URL.
// On errors returns HTTP 4** with an error message.
func (impl *repoImpl) handlePackageDownload(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	// Parse the path. The router is too simplistic to parse such paths.
	pkg, version, err := parseDownloadPath(ctx.Params.ByName("path"))
	if err != nil {
		return err
	}

	// Resolve the version into a concrete instance. This also does rigorous
	// argument validation, ACL checks and verifies the resulting instance exists.
	inst, err := impl.ResolveVersion(c, &repopb.ResolveVersionRequest{
		Package: pkg,
		Version: version,
	})
	if err != nil {
		return err
	}

	// Put resolved instance ID into the response headers. This may be useful when
	// debugging fetches.
	w.Header().Set(cipdInstanceHeader, common.ObjectRefToInstanceID(inst.Instance))

	// Generate a name for the file based on the last two components of the
	// package name. This name is used by browsers when downloading the file.
	name := ""
	chunks := strings.Split(pkg, "/")
	if len(chunks) > 1 {
		name = fmt.Sprintf("%s-%s", chunks[len(chunks)-2], chunks[len(chunks)-1])
	} else {
		name = chunks[0]
	}

	// Ask CAS for a signed URL to the package and redirect there.
	url, err := impl.cas.GetObjectURL(c, &caspb.GetObjectURLRequest{
		Object:           inst.Instance,
		DownloadFilename: name + ".zip",
	})
	if err == nil {
		http.Redirect(w, r, url.SignedUrl, http.StatusFound)
	}
	return err
}

// handleBootstrapDownload redirects to an extracted bootstrap file in GCS.
//
// GET /bootstrap/<package>/+/<version>.
//
// Where:
//
//	package: a CIPD package name (e.g. "a/b/c/linux-amd64").
//	version: a package version identifier (instance ID, a ref or a tag).
//
// Works only with bootstrap packages (per bootstrap.cfg file). See also
// DescribeBootstrapBundle.
//
// On success issues HTTP 302 redirect to the signed Google Storage URL.
// On errors returns HTTP 4** with an error message.
func (impl *repoImpl) handleBootstrapDownload(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	// Parse the path. The router is too simplistic to parse such paths.
	pkg, version, err := parseDownloadPath(ctx.Params.ByName("path"))
	if err != nil {
		return err
	}

	// Split the full package name into the prefix and variant.
	prefix, variant := "", ""
	if idx := strings.LastIndex(pkg, "/"); idx != -1 {
		prefix, variant = pkg[:idx], pkg[idx+1:]
	} else {
		prefix, variant = "/", pkg
	}

	// Grab information about the bootstrap file. This does ACL checks, version
	// resolution, etc.
	bundle, err := impl.DescribeBootstrapBundle(c, &repopb.DescribeBootstrapBundleRequest{
		Prefix:   prefix,
		Variants: []string{variant},
		Version:  version,
	})
	if err != nil {
		return err
	}

	// If DescribeBootstrapBundle succeeded, it *must* have one valid entry.
	if len(bundle.Files) != 1 {
		panic(fmt.Sprintf("unexpected BootstrapFile entries: %v", bundle.Files))
	}
	bf := bundle.Files[0]
	if bf.Status.GetCode() != 0 {
		panic(fmt.Sprintf("unexpected non-OK code: %s", bf.Status))
	}

	// Put resolved instance ID and file hash into the response headers. This may
	// be useful when debugging fetches.
	w.Header().Set(cipdInstanceHeader, common.ObjectRefToInstanceID(bf.Instance))
	w.Header().Set(cipdDigestHeader, fmt.Sprintf("%s:%s", bf.File.HashAlgo, bf.File.HexDigest))

	// Ask CAS for a signed URL to the extracted file and redirect there.
	url, err := impl.cas.GetObjectURL(c, &caspb.GetObjectURLRequest{
		Object:           bf.File,
		DownloadFilename: bf.Name,
	})
	if err == nil {
		http.Redirect(w, r, url.SignedUrl, http.StatusFound)
	}
	return err
}

// handleLegacyClientInfo is a legacy handler for an RPC replaced by /client
// endpoint.
//
// It returns information about a CIPD client package and a signed URL to fetch
// the client binary.
//
// GET /_ah/api/repo/v1/client?package_name=...&instance_id=...
//
// Where:
//
//	package_name: full name of a CIPD client package.
//	instance_id: a hex digest with instance ID.
//
// Returns:
//
//	{
//	  "status": "...",
//	  "error_message": "...",
//	  "client_binary": {
//	    "file_name": "cipd",
//	    "sha1": "...",
//	    "fetch_url": "...",
//	    "size": "..."
//	  },
//	  "instance": {
//	    "package_name": "...",
//	    "instance_id": "...",
//	    "registered_by": "...",
//	    "registered_ts": "<int64 timestamp in microseconds>"
//	  }
//	}
func (impl *repoImpl) handleLegacyClientInfo(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	iid := r.FormValue("instance_id")
	if err := common.ValidateInstanceID(iid, common.KnownHash); err != nil {
		return grpcutil.InvalidArgumentTag.Apply(errors.Fmt("bad instance_id: %w", err))
	}

	desc, err := impl.DescribeClient(c, &repopb.DescribeClientRequest{
		Package:  r.FormValue("package_name"),
		Instance: common.InstanceIDToObjectRef(iid),
	})

	switch status.Code(err) {
	case codes.OK:
		return replyWithJSON(w, map[string]any{
			"status":   "SUCCESS",
			"instance": (&legacyInstance{}).FromInstance(desc.Instance),
			"client_binary": map[string]string{
				"file_name": processing.GetClientBinaryName(desc.Instance.Package),
				"sha1":      desc.LegacySha1,
				"fetch_url": desc.ClientBinary.SignedUrl,
				"size":      fmt.Sprintf("%d", desc.ClientSize),
			},
		})
	case codes.NotFound:
		return replyWithError(w, "INSTANCE_NOT_FOUND", "%s", status.Convert(err).Message())
	case codes.FailedPrecondition:
		return replyWithError(w, "NOT_EXTRACTED_YET", "the client binary is not extracted yet, try later")
	case codes.Aborted:
		return replyWithError(w, "ERROR", "the client binary is not available: %s", status.Convert(err).Message())
	default:
		return err // the legacy client recognizes other codes just fine
	}
}

// handleLegacyInstance is a legacy handler for an RPC replaced by
// DescribeInstance and GetInstanceURL.
//
// It returns information about an instance and a signed URL to fetch it.
//
// GET /_ah/api/repo/v1/instance?package_name=...&instance_id=...
//
// Where:
//
//	package_name: full name of a package.
//	instance_id: a hex digest with instance ID.
//
// Returns:
//
//	{
//	  "status": "...",
//	  "error_message": "...",
//	  "fetch_url": "...",
//	  "instance": {
//	    "package_name": "...",
//	    "instance_id": "...",
//	    "registered_by": "...",
//	    "registered_ts": "<int64 timestamp in microseconds>"
//	  }
//	}
func (impl *repoImpl) handleLegacyInstance(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	iid := r.FormValue("instance_id")
	if err := common.ValidateInstanceID(iid, common.KnownHash); err != nil {
		return grpcutil.InvalidArgumentTag.Apply(errors.

			// This checks the request format, ACLs, verifies the instance exists and
			// returns info about it.
			Fmt("bad instance_id: %w", err))
	}

	inst, err := impl.DescribeInstance(c, &repopb.DescribeInstanceRequest{
		Package:  r.FormValue("package_name"),
		Instance: common.InstanceIDToObjectRef(iid),
	})

	var signedURL *caspb.ObjectURL
	if err == nil {
		// Here we know the instance exists and the caller has access to it, so just
		// ask the CAS for an URL directly instead of using impl.GetInstanceURL,
		// which will needlessly recheck ACLs and instance presence.
		signedURL, err = impl.cas.GetObjectURL(c, &caspb.GetObjectURLRequest{
			Object: inst.Instance.Instance,
		})
	}

	switch status.Code(err) {
	case codes.OK:
		return replyWithJSON(w, map[string]any{
			"status":    "SUCCESS",
			"fetch_url": signedURL.SignedUrl,
			"instance":  (&legacyInstance{}).FromInstance(inst.Instance),
		})
	case codes.NotFound:
		return replyWithError(w, "INSTANCE_NOT_FOUND", "%s", status.Convert(err).Message())
	default:
		return err // the legacy client recognizes other codes just fine
	}
}

// handleLegacyResolve is a legacy handler for ResolveVersion RPC.
//
// GET /_ah/api/repo/v1/instance/resolve?package_name=...&version=...
//
// Where:
//
//	package_name: full name of a package.
//	version: a package version identifier (instance ID, a ref or a tag).
//
// Returns:
//
//	{
//	  "status": "...",
//	  "error_message": "...",
//	  "instance_id": "..."
//	}
func (impl *repoImpl) handleLegacyResolve(ctx *router.Context) error {
	c, r, w := ctx.Request.Context(), ctx.Request, ctx.Writer

	resp, err := impl.ResolveVersion(c, &repopb.ResolveVersionRequest{
		Package: r.FormValue("package_name"),
		Version: r.FormValue("version"),
	})

	switch status.Code(err) {
	case codes.OK:
		return replyWithJSON(w, map[string]string{
			"status":      "SUCCESS",
			"instance_id": common.ObjectRefToInstanceID(resp.Instance),
		})
	case codes.NotFound:
		return replyWithError(w, "INSTANCE_NOT_FOUND", "%s", status.Convert(err).Message())
	case codes.FailedPrecondition:
		return replyWithError(w, "AMBIGUOUS_VERSION", "%s", status.Convert(err).Message())
	default:
		return err // the legacy client recognizes other codes just fine
	}
}

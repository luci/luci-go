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
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/cipd/appengine/impl/metadata"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/repo/tasks"
	"go.chromium.org/luci/cipd/common"
)

// Public returns publicly exposed implementation of cipd.Repository service.
//
// It checks ACLs.
func Public(internalCAS cas.StorageServer, d *tq.Dispatcher) api.RepositoryServer {
	impl := &repoImpl{
		tq:   d,
		meta: metadata.GetStorage(),
		cas:  internalCAS,
	}
	impl.registerTasks()
	return impl
}

// repoImpl implements api.RepositoryServer.
type repoImpl struct {
	tq *tq.Dispatcher

	meta metadata.Storage  // storage for package prefix metadata
	cas  cas.StorageServer // non-ACLed storage for instance package files
}

// registerTasks adds tasks to the tq Dispatcher.
func (impl *repoImpl) registerTasks() {
	// See queue.yaml for "run-processors" task queue definition.
	impl.tq.RegisterTask(&tasks.RunProcessors{}, func(c context.Context, m proto.Message) error {
		return impl.runProcessorsTask(c, m.(*tasks.RunProcessors))
	}, "run-processors", nil)
}

////////////////////////////////////////////////////////////////////////////////
// Prefix metadata RPC methods + related helpers including ACL checks.

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
// Returns grpc PermissionDenied error if the caller doesn't have the requested
// role. The error message depends on whether caller has READER role or not
// (readers see more details).
//
// Fetches and returns metadata of the prefix and all parent prefixes as a side
// effect.
func (impl *repoImpl) checkRole(c context.Context, prefix string, role api.Role) ([]*api.PrefixMetadata, error) {
	metas, err := impl.meta.GetMetadata(c, prefix)
	if err != nil {
		return nil, err
	}

	switch yes, err := hasRole(c, metas, role); {
	case err != nil:
		return nil, err
	case yes:
		return metas, nil
	case role == api.Role_READER: // was checking for a reader, and caller is not
		return nil, noAccessErr(prefix)
	}

	// We end up here if role is something other than READER, and the caller
	// doesn't have it. Maybe caller IS a reader, then we can give more concrete
	// error message.
	switch yes, err := hasRole(c, metas, api.Role_READER); {
	case err != nil:
		return nil, err
	case yes:
		return nil, status.Errorf(codes.PermissionDenied, "caller has no required %s role in prefix %q", role, prefix)
	default:
		return nil, noAccessErr(prefix)
	}
}

// noAccessErr produces a grpc error saying that the given prefix doesn't
// exist or the caller has no access to it. This is generic error message that
// should not give away prefix presence to non-readers.
func noAccessErr(prefix string) error {
	return status.Errorf(codes.PermissionDenied, "prefix %q doesn't exist or the caller is not allowed to see it", prefix)
}

// noMetadataErr produces a grpc error saying that the given prefix doesn't have
// metadata attached.
func noMetadataErr(prefix string) error {
	return status.Errorf(codes.NotFound, "prefix %q has no metadata", prefix)
}

////////////////////////////////////////////////////////////////////////////////
// Package instance registration.

// RegisterInstance implements the corresponding RPC method, see the proto doc.
func (impl *repoImpl) RegisterInstance(c context.Context, r *api.Instance) (resp *api.RegisterInstanceResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(c, err) }()

	// Validate the request format.
	if err := common.ValidatePackageName(r.Package); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'package' - %s", err)
	}
	if err := cas.ValidateObjectRef(r.Instance); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad 'instance' - %s", err)
	}

	// Check ACLs.
	if _, err := impl.checkRole(c, r.Package, api.Role_WRITER); err != nil {
		return nil, err
	}

	// Is such instance already registered?
	instance := &model.Instance{
		Package:    model.PackageKey(c, r.Package),
		InstanceID: model.ObjectRefToInstanceID(r.Instance),
	}
	switch err := datastore.Get(c, instance); {
	case err == nil:
		return &api.RegisterInstanceResponse{
			Status:   api.RegistrationStatus_ALREADY_REGISTERED,
			Instance: instance.Proto(),
		}, nil
	case err != datastore.ErrNoSuchEntity:
		return nil, errors.Annotate(err, "failed to fetch the instance entity").Err()
	}

	// Attempt to start a new upload session. This will fail with ALREADY_EXISTS
	// if such object is already in the storage. This is expected (it means the
	// client has uploaded the object already and we should just register the
	// instance right away).
	uploadOp, err := impl.cas.BeginUpload(c, &api.BeginUploadRequest{
		Object: r.Instance,
	})
	switch code := grpc.Code(err); {
	case code == codes.AlreadyExists:
		break // the object is already there
	case code == codes.OK:
		// The object is not in the storage and we have just started the upload. Let
		// the client finish it.
		return &api.RegisterInstanceResponse{
			Status:   api.RegistrationStatus_NOT_UPLOADED,
			UploadOp: uploadOp,
		}, nil
	default:
		return nil, errors.Annotate(err, "failed to initiate an upload op (code %s)", code).Err()
	}

	// The instance is already in the CAS storage. Register it in the repository.
	registered, instance, err := model.RegisterInstance(c, &model.Instance{
		Package:      model.PackageKey(c, r.Package),
		InstanceID:   model.ObjectRefToInstanceID(r.Instance),
		RegisteredBy: string(auth.CurrentIdentity(c)),
		RegisteredTs: clock.Now(c).UTC(),
	}, impl.onInstanceRegistration)
	if err != nil {
		return nil, errors.Annotate(err, "failed to register the instance").Err()
	}

	resp = &api.RegisterInstanceResponse{Instance: instance.Proto()}
	if registered {
		resp.Status = api.RegistrationStatus_REGISTERED
	} else {
		resp.Status = api.RegistrationStatus_ALREADY_REGISTERED
	}
	return
}

// onInstanceRegistration is called in a txn when registering an instance.
func (impl *repoImpl) onInstanceRegistration(c context.Context, inst *model.Instance) error {
	// TODO(vadimsh): Enqueue tasks.RunProcessors task.
	return nil
}

// runProcessorTask executes a post-upload processing step.
//
// Returning a transient error here causes the task queue service to retry the
// task.
func (impl *repoImpl) runProcessorsTask(c context.Context, t *tasks.RunProcessors) error {
	// TODO(vadimsh): Implement.
	return nil
}

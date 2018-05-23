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

package cipd

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/grpc/prpc"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

var errNoV2Impl = errors.New("this call is not yet implemented in v2 protocol")

// prpcRemoteImpl implements v1 'remote' interface using v2 protocol.
//
// It exists temporarily during the transition from v1 to v2 protocol. Once
// v2 is fully implemented and becomes the default, v1 implementation of the
// 'remote' interface will be deleted, and the interface itself will be adjusted
// to  match v2 protocol better (or deleted completely, since pRPC-level
// interface is good enough on its own).
type prpcRemoteImpl struct {
	serviceURL string
	userAgent  string
	client     *http.Client

	cas  api.StorageClient
	repo api.RepositoryClient
}

func (r *prpcRemoteImpl) init() error {
	// Note: serviceURL is ALWAYS "http(s)://<host>" here, as setup by NewClient.
	parsed, err := url.Parse(r.serviceURL)
	if err != nil {
		panic(err)
	}
	prpcC := &prpc.Client{
		C:    r.client,
		Host: parsed.Host,
		Options: &prpc.Options{
			UserAgent: r.userAgent,
			Insecure:  parsed.Scheme == "http", // for testing with local dev server
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:   time.Second,
						Retries: 5,
					},
				}
			},
		},
	}

	r.cas = api.NewStoragePRPCClient(prpcC)
	r.repo = api.NewRepositoryPRPCClient(prpcC)
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// ACLs.

var legacyRoles = map[string]api.Role{
	"READER": api.Role_READER,
	"WRITER": api.Role_WRITER,
	"OWNER":  api.Role_OWNER,
}

func grantRole(m *api.PrefixMetadata, role api.Role, principal string) bool {
	var roleAcl *api.PrefixMetadata_ACL
	for _, acl := range m.Acls {
		if acl.Role != role {
			continue
		}
		for _, p := range acl.Principals {
			if p == principal {
				return false // already have it
			}
		}
		roleAcl = acl
	}

	if roleAcl != nil {
		// Append to the existing ACL.
		roleAcl.Principals = append(roleAcl.Principals, principal)
	} else {
		// Add new ACL for this role, this is the first one.
		m.Acls = append(m.Acls, &api.PrefixMetadata_ACL{
			Role:       role,
			Principals: []string{principal},
		})
	}

	return true
}

func revokeRole(m *api.PrefixMetadata, role api.Role, principal string) bool {
	dirty := false
	for _, acl := range m.Acls {
		if acl.Role != role {
			continue
		}
		filtered := acl.Principals[:0]
		for _, p := range acl.Principals {
			if p != principal {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) != len(acl.Principals) {
			acl.Principals = filtered
			dirty = true
		}
	}

	if !dirty {
		return false
	}

	// Kick out empty ACL entries.
	acls := m.Acls[:0]
	for _, acl := range m.Acls {
		if len(acl.Principals) != 0 {
			acls = append(acls, acl)
		}
	}
	if len(acls) == 0 {
		m.Acls = nil
	} else {
		m.Acls = acls
	}
	return true
}

func (r *prpcRemoteImpl) fetchACL(ctx context.Context, packagePath string) ([]PackageACL, error) {
	resp, err := r.repo.GetInheritedPrefixMetadata(ctx, &api.PrefixRequest{
		Prefix: packagePath,
	})
	if err != nil {
		return nil, err
	}
	var out []PackageACL
	for _, p := range resp.PerPrefixMetadata {
		var acls []PackageACL
		for _, acl := range p.Acls {
			role := acl.Role.String()
			found := false
			for i, existing := range acls {
				if existing.Role == role {
					acls[i].Principals = append(acls[i].Principals, acl.Principals...)
					found = true
					break
				}
			}
			if !found {
				acls = append(acls, PackageACL{
					PackagePath: p.Prefix,
					Role:        role,
					Principals:  acl.Principals,
					ModifiedBy:  p.UpdateUser,
					ModifiedTs:  UnixTime(google.TimeFromProto(p.UpdateTime)),
				})
			}
		}
		out = append(out, acls...)
	}
	return out, nil
}

func (r *prpcRemoteImpl) modifyACL(ctx context.Context, packagePath string, changes []PackageACLChange) error {
	// Fetch existing metadata, if any.
	meta, err := r.repo.GetPrefixMetadata(ctx, &api.PrefixRequest{
		Prefix: packagePath,
	}, prpc.ExpectedCode(codes.NotFound))
	if code := grpc.Code(err); code != codes.OK && code != codes.NotFound {
		return err
	}

	// Construct new empty metadata for codes.NotFound.
	if meta == nil {
		meta = &api.PrefixMetadata{Prefix: packagePath}
	}

	// Apply mutations.
	dirty := false
	for _, ch := range changes {
		role, ok := legacyRoles[ch.Role]
		if !ok {
			// Just log and ignore. Aborting with error here breaks 'acl-edit -revoke'
			// functionality, since it always tries to revoke all possible roles,
			// including unsupported COUNTER_WRITER.
			logging.Warningf(ctx, "Ignoring role %q not supported in v2", ch.Role)
			continue
		}
		changed := false
		switch ch.Action {
		case GrantRole:
			changed = grantRole(meta, role, ch.Principal)
		case RevokeRole:
			changed = revokeRole(meta, role, ch.Principal)
		default:
			return fmt.Errorf("unrecognized PackageACLChangeAction %q", ch.Action)
		}
		dirty = dirty || changed
	}

	if !dirty {
		return nil
	}

	// Store the new metadata. This call will check meta.Fingerprint.
	_, err = r.repo.UpdatePrefixMetadata(ctx, meta)
	return err
}

////////////////////////////////////////////////////////////////////////////////
// Upload.

func (r *prpcRemoteImpl) initiateUpload(ctx context.Context, sha1 string) (*UploadSession, error) {
	op, err := r.cas.BeginUpload(ctx, &api.BeginUploadRequest{
		Object: &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: sha1,
		},
	}, prpc.ExpectedCode(codes.AlreadyExists))
	switch grpc.Code(err) {
	case codes.OK:
		return &UploadSession{op.OperationId, op.UploadUrl}, nil
	case codes.AlreadyExists:
		return nil, nil
	default:
		return nil, err
	}
}

func (r *prpcRemoteImpl) finalizeUpload(ctx context.Context, sessionID string) (bool, error) {
	op, err := r.cas.FinishUpload(ctx, &api.FinishUploadRequest{
		UploadOperationId: sessionID,
	})
	if err != nil {
		return false, err
	}
	switch op.Status {
	case api.UploadStatus_UPLOADING, api.UploadStatus_VERIFYING:
		return false, nil // still verifying
	case api.UploadStatus_PUBLISHED:
		return true, nil // verified!
	case api.UploadStatus_ERRORED:
		return false, errors.New(op.ErrorMessage)
	default:
		return false, fmt.Errorf("unrecognized upload operation status %s", op.Status)
	}
}

func (r *prpcRemoteImpl) registerInstance(ctx context.Context, pin common.Pin) (*registerInstanceResponse, error) {
	resp, err := r.repo.RegisterInstance(ctx, &api.Instance{
		Package: pin.PackageName,
		Instance: &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: pin.InstanceID,
		},
	})
	if err != nil {
		return nil, err
	}
	switch resp.Status {
	case api.RegistrationStatus_REGISTERED, api.RegistrationStatus_ALREADY_REGISTERED:
		return &registerInstanceResponse{
			alreadyRegistered: resp.Status == api.RegistrationStatus_ALREADY_REGISTERED,
			registeredBy:      resp.Instance.RegisteredBy,
			registeredTs:      google.TimeFromProto(resp.Instance.RegisteredTs),
		}, nil
	case api.RegistrationStatus_NOT_UPLOADED:
		return &registerInstanceResponse{
			uploadSession: &UploadSession{resp.UploadOp.OperationId, resp.UploadOp.UploadUrl},
		}, nil
	default:
		return nil, fmt.Errorf("unrecognized package registration status %s", resp.Status)
	}
}

////////////////////////////////////////////////////////////////////////////////
// Fetching.

func (r *prpcRemoteImpl) resolveVersion(ctx context.Context, packageName, version string) (common.Pin, error) {
	return common.Pin{}, errNoV2Impl
}

func (r *prpcRemoteImpl) fetchPackage(ctx context.Context, packageName string, withRefs bool) (*fetchPackageResponse, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) fetchInstance(ctx context.Context, pin common.Pin) (*fetchInstanceResponse, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) fetchClientBinaryInfo(ctx context.Context, pin common.Pin) (*fetchClientBinaryInfoResponse, error) {
	return nil, errNoV2Impl
}

////////////////////////////////////////////////////////////////////////////////
// Refs and tags.

func (r *prpcRemoteImpl) setRef(ctx context.Context, ref string, pin common.Pin) error {
	return errNoV2Impl
}

func (r *prpcRemoteImpl) attachTags(ctx context.Context, pin common.Pin, tags []string) error {
	return errNoV2Impl
}

func (r *prpcRemoteImpl) fetchTags(ctx context.Context, pin common.Pin, tags []string) ([]TagInfo, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) fetchRefs(ctx context.Context, pin common.Pin, refs []string) ([]RefInfo, error) {
	return nil, errNoV2Impl
}

////////////////////////////////////////////////////////////////////////////////
// Misc.

func (r *prpcRemoteImpl) listPackages(ctx context.Context, path string, recursive, showHidden bool) ([]string, []string, error) {
	return nil, nil, errNoV2Impl
}

func (r *prpcRemoteImpl) searchInstances(ctx context.Context, tag, packageName string) (common.PinSlice, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) listInstances(ctx context.Context, packageName string, limit int, cursor string) (*listInstancesResponse, error) {
	return nil, errNoV2Impl
}

func (r *prpcRemoteImpl) deletePackage(ctx context.Context, packageName string) error {
	return errNoV2Impl
}

func (r *prpcRemoteImpl) incrementCounter(ctx context.Context, pin common.Pin, counter string, delta int) error {
	return errNoV2Impl
}

func (r *prpcRemoteImpl) readCounter(ctx context.Context, pin common.Pin, counter string) (Counter, error) {
	return Counter{}, errNoV2Impl
}

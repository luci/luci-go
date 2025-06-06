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

package model

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cipd/common"
)

// ResolveVersion takes a version identifier (an instance ID, a ref or a tag)
// and resolves it into a concrete package instance, ensuring it exists.
//
// Returns gRPC-tagged errors:
//
//	InvalidArgument if the version string format is invalid.
//	NotFound if there's no such package or such version.
//	FailedPrecondition if the tag resolves to multiple instances.
func ResolveVersion(ctx context.Context, pkg, version string) (*Instance, error) {
	// Pick a resolution method based on the format of the version string.
	var iid string
	var err error
	switch {
	case common.ValidateInstanceID(version, common.KnownHash) == nil:
		iid = version
	case common.ValidatePackageRef(version) == nil:
		var ref *Ref
		if ref, err = GetRef(ctx, pkg, version); err == nil {
			iid = ref.InstanceID
		}
	case common.ValidateInstanceTag(version) == nil:
		iid, err = ResolveTag(ctx, pkg, common.MustParseInstanceTag(version))
	default:
		return nil, grpcutil.InvalidArgumentTag.Apply(errors.New("not a valid version identifier"))
	}

	if err != nil {
		// If there's no such ref or tag, maybe the package is missing completely.
		if grpcutil.Code(err) == codes.NotFound {
			if pkgErr := CheckPackageExists(ctx, pkg); pkgErr != nil {
				return nil, pkgErr
			}
		}
		return nil, err
	}

	// Verify the instance exists and fetch its details. This is particularly
	// important for the case when 'version' is raw instance ID already. For
	// cases of tags and refs, the instance MUST exist per checks we do in SetRef
	// and AttachTags, but being defensive here doesn't hurt.
	inst := &Instance{
		InstanceID: iid,
		Package:    PackageKey(ctx, pkg),
	}
	if err := CheckInstanceExists(ctx, inst); err != nil {
		return nil, err
	}
	return inst, nil
}

// Copyright 2026 The LUCI Authors.
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

package perm

import (
	"go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/rpcacl"
)

var (
	readerPermOpts = permOpts{
		role:  repopb.Role_READER,
		group: PrefixesViewers,
	}
	writerPermOpts = permOpts{
		role:    repopb.Role_WRITER,
		isWrite: true,
	}
	ownerPermOpts = permOpts{
		role:    repopb.Role_OWNER,
		isWrite: true,
	}
)

// Realm permissions that CIPD will check.
var (
	// READER permissions
	ListInstances          = definePerm("cipd.packages.listInstances", readerPermOpts)
	ListRefs               = definePerm("cipd.packages.listRefs", readerPermOpts)
	DiagnoseACL            = definePerm("cipd.packages.diagnoseACL", readerPermOpts)
	ReadInstanceData       = definePerm("cipd.packages.readInstanceData", readerPermOpts)
	ReadInstanceInfo       = definePerm("cipd.packages.readInstanceInfo", readerPermOpts)
	ReadInstanceProcessors = definePerm("cipd.packages.readInstanceProcessors", readerPermOpts)
	ReadInstanceTags       = definePerm("cipd.packages.readInstanceTags", readerPermOpts)
	ResolveVersion         = definePerm("cipd.packages.resolveVersion", readerPermOpts)
	ReadInstanceMetadata   = definePerm("cipd.packages.readInstanceMetadata", readerPermOpts)

	// WRITER permissions
	DetachRef             = defineBindable("cipd.packages.detachRef", attrRefNameKey, writerPermOpts)
	WriteInstance         = definePerm("cipd.packages.writeInstance", writerPermOpts)
	WriteInstanceMetadata = defineBindable("cipd.packages.writeInstanceMetadata", attrMetadataKey, writerPermOpts)
	WriteInstanceTag      = defineBindable("cipd.packages.writeInstanceTag", attrTagKey, writerPermOpts)
	WriteRef              = defineBindable("cipd.packages.writeRef", attrRefNameKey, writerPermOpts)

	// OWNER permissions
	DetachInstanceMetadata = defineBindable("cipd.packages.detachInstanceMetadata", attrMetadataKey, ownerPermOpts)
	DetachInstanceTag      = defineBindable("cipd.packages.detachInstanceTag", attrTagKey, ownerPermOpts)
	HidePackage            = definePerm("cipd.packages.hide", ownerPermOpts)

	// NOTE: This permission is implied by OWNER, but the PrefixesViewers group
	// (typically associated with READER) is granted this.
	ReadLegacyRoles = definePerm("cipd.packages.readLegacyRoles", permOpts{
		role:  repopb.Role_OWNER,
		group: PrefixesViewers,
	})

	// DeletePackage is not a real realm permission, but is here for API uniformity.
	DeletePackage = definePerm("cipd.packages.delete", permOpts{
		isWrite:   true,
		group:     rpcacl.AdminGroup,
		groupOnly: true,
	})

	// AllPermissions is the set of all Grantables defined in this package.
	AllPermissions = map[Grantable]struct{}{}
)

// AllPermissionsSet returns [AllPermissions] as a [Checkable] [Set].
//
// Note that for bindable permissions, this contains their unconditional
// variants.
func AllPermissionsSet() Set {
	ret := make(Set, len(AllPermissions))
	for perm := range AllPermissions {
		ret.Add(Permission{core: perm.(*corePermission)})
	}
	return ret
}

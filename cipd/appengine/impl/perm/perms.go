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

// Realm permissions that CIPD will check.
var (
	// READER permissions
	ListInstances = definePerm("cipd.packages.listInstances").
			withRole(repopb.Role_READER).
			withGroup(PrefixesViewers)
	ListRefs = definePerm("cipd.packages.listRefs").
			withRole(repopb.Role_READER).
			withGroup(PrefixesViewers)
	DiagnoseACL = definePerm("cipd.packages.diagnoseACL").
			withRole(repopb.Role_READER).
			withGroup(PrefixesViewers)
	ReadInstanceData = definePerm("cipd.packages.readInstanceData").
				withRole(repopb.Role_READER).
				withGroup(PrefixesViewers)
	ReadInstanceInfo = definePerm("cipd.packages.readInstanceInfo").
				withRole(repopb.Role_READER).
				withGroup(PrefixesViewers)
	ReadInstanceProcessors = definePerm("cipd.packages.readInstanceProcessors").
				withRole(repopb.Role_READER).
				withGroup(PrefixesViewers)
	ReadInstanceTags = definePerm("cipd.packages.readInstanceTags").
				withRole(repopb.Role_READER).
				withGroup(PrefixesViewers)
	ResolveVersion = definePerm("cipd.packages.resolveVersion").
			withRole(repopb.Role_READER).
			withGroup(PrefixesViewers)
	ReadInstanceMetadata = definePerm("cipd.packages.readInstanceMetadata").
				withRole(repopb.Role_READER).
				withGroup(PrefixesViewers)

	// WRITER permissions
	DetachRef = definePerm("cipd.packages.detachRef").asWrite().
			withRole(repopb.Role_WRITER).
			withAttr("cipd.package.refName")
	WriteInstance = definePerm("cipd.packages.writeInstance").asWrite().
			withRole(repopb.Role_WRITER)
	WriteInstanceMetadata = definePerm("cipd.packages.writeInstanceMetadata").asWrite().
				withRole(repopb.Role_WRITER).
				withAttr("cipd.instance.metadataKey")
	WriteInstanceTag = definePerm("cipd.packages.writeInstanceTag").asWrite().
				withRole(repopb.Role_WRITER).
				withAttr("cipd.instance.tagKey")
	WriteRef = definePerm("cipd.packages.writeRef").asWrite().
			withRole(repopb.Role_WRITER).
			withAttr("cipd.package.refName")

	// OWNER permissions
	DetachInstanceMetadata = definePerm("cipd.packages.detachInstanceMetadata").asWrite().
				withRole(repopb.Role_OWNER).
				withAttr("cipd.instance.metadataKey")
	DetachInstanceTag = definePerm("cipd.packages.detachInstanceTag").asWrite().
				withRole(repopb.Role_OWNER).
				withAttr("cipd.instance.tagKey")
	HidePackage = definePerm("cipd.packages.hide").asWrite().
			withRole(repopb.Role_OWNER)

	// NOTE: This permission is implied by OWNER, but the PrefixesViewers group
	// (typically associated with READER) is granted this.
	ReadLegacyRoles = definePerm("cipd.packages.readLegacyRoles").
			withRole(repopb.Role_OWNER).
			withGroup(PrefixesViewers)

	// DeletePackage is not a real realm permission, but is here for API uniformity.
	DeletePackage = definePerm("cipd.packages.delete").
			groupOnly(rpcacl.AdminGroup).
			asWrite()

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

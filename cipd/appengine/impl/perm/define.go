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
	"go.chromium.org/luci/server/auth/realms"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

type permOpts struct {
	role      repopb.Role
	group     string
	groupOnly bool
	isWrite   bool
}

func (opts permOpts) register(permission string, attrKey attrKey) *corePermission {
	if permission == "" {
		panic("permission is required")
	}
	if opts.groupOnly && opts.group == "" {
		panic("groupOnly requires a group")
	}
	if opts.groupOnly && opts.role != 0 {
		panic("groupOnly requires no role")
	}
	core := &corePermission{
		perm:      realms.RegisterPermission(permission),
		groupOnly: opts.groupOnly,
		isWrite:   opts.isWrite,
		role:      opts.role,
		group:     opts.group,
		attrKey:   attrKey,
	}
	AllPermissions[core] = struct{}{}
	return core
}

type attrKey string

const (
	attrMetadataKey attrKey = `cipd.instance.metadataKey`
	attrTagKey      attrKey = `cipd.instance.tagKey`
	attrRefNameKey  attrKey = `cipd.package.refName`
)

var allAttrKeys = map[attrKey]struct{}{
	attrMetadataKey: {},
	attrTagKey:      {},
	attrRefNameKey:  {},
}

// definePerm registers `permission` as a realms permission, and returns a new
// [Permission].
func definePerm(permission string, opts permOpts) Permission {
	return Permission{core: opts.register(permission, "")}
}

// defineBindable registers `permission` as a realms permission, and returns a
// new [Bindable] with attrKey.
func defineBindable(permission string, attrKey attrKey, opts permOpts) Bindable {
	if _, ok := allAttrKeys[attrKey]; !ok {
		panic("bad attrKey")
	}
	return Bindable{core: opts.register(permission, attrKey)}
}

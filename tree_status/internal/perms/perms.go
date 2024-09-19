// Copyright 2024 The LUCI Authors.
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

// Package perms defines permissions used to control access to Tree Status
// resources, and related methods.
package perms

import (
	"go.chromium.org/luci/server/auth/realms"
)

// All permissions in this file are checked against "<luciproject>:<subrealm>"
// realm, where the <luciproject> refers to the primary project of the tree.
// If subrealm is not specified, default to be "@project".
var (
	// PermGetStatusLimited allows users to get status, but does not allow seeing PII (username).
	PermGetStatusLimited = realms.RegisterPermission("treestatus.status.getLimited")

	// PermListStatusLimited allows users to list status, but does not allow seeing PII (username).
	PermListStatusLimited = realms.RegisterPermission("treestatus.status.listLimited")

	// PermGetStatus allows users to get status, including PII.
	// Note that the user also needs to be a Googler, in addition to having the permission.
	PermGetStatus = realms.RegisterPermission("treestatus.status.get")

	// PermListStatus allows users to list status, including PII.
	// Note that the user also needs to be a Googler, in addition to having the permission.
	PermListStatus = realms.RegisterPermission("treestatus.status.list")

	// PermCreateStatus allows users to create status.
	PermCreateStatus = realms.RegisterPermission("treestatus.status.create")

	// PermListTree allows users to list trees.
	PermListTree = realms.RegisterPermission("treestatus.trees.list")

	// PermGetTree allows users to get a tree.
	PermGetTree = realms.RegisterPermission("treestatus.trees.get")
)

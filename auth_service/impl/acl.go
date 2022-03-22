// Copyright 2021 The LUCI Authors.
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

package impl

import (
	"go.chromium.org/luci/server/auth/rpcacl"
)

// ServiceAccessGroup defines a group whose members are allowed to use the
// service API and UI.
//
// It nominally grants read access only. Permission to modify individual
// groups is controlled by "owners" group field.
const ServiceAccessGroup = "auth-service-access"

// AuthorizeRPCAccess is a gRPC unary interceptor that checks the caller is
// in the group that grants access to the auth service API.
var AuthorizeRPCAccess = rpcacl.Interceptor(rpcacl.Map{
	// Discovery API is used by the RPC Explorer to show the list of APIs. It just
	// returns the proto descriptors already available through the public source
	// code.
	"/discovery.Discovery/*": rpcacl.All,

	// GetSelf just checks credentials and doesn't access any data.
	"/auth.service.Accounts/GetSelf": rpcacl.All,

	// All methods to work with groups require authorization.
	"/auth.service.Groups/*": ServiceAccessGroup,

	// All methods to work with allowlists require authorization.
	"/auth.service.Allowlists/*": ServiceAccessGroup,

	// Internals are used by the UI which is accessible only to authorized users.
	"/auth.internals.Internals/*": ServiceAccessGroup,
})

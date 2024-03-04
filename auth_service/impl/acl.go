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
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/rpcacl"

	"go.chromium.org/luci/auth_service/impl/model"
)

// AuthorizeRPCAccess is a gRPC server interceptor that checks the caller is
// in the group that grants access to the auth service API.
var AuthorizeRPCAccess = rpcacl.Interceptor(rpcacl.Map{
	// Discovery API is used by the RPC Explorer to show the list of APIs. It just
	// returns the proto descriptors already available through the public source
	// code.
	"/discovery.Discovery/*": rpcacl.All,

	// GetSelf just checks credentials and doesn't access any data.
	"/auth.service.Accounts/GetSelf": rpcacl.All,

	// All methods to work with groups require authorization.
	"/auth.service.Groups/*": authdb.AuthServiceAccessGroup,

	// Only administrators can create groups.
	"/auth.service.Groups/CreateGroup": model.AdminGroup,

	// All methods to work with allowlists require authorization.
	"/auth.service.Allowlists/*": authdb.AuthServiceAccessGroup,

	// All methods to work with AuthDB require authorization.
	"/auth.service.AuthDB/*": authdb.AuthServiceAccessGroup,

	// All methods to work with ChangeLogs require authorization.
	"/auth.service.ChangeLogs/*": authdb.AuthServiceAccessGroup,

	// Internals are used by the UI which is accessible only to authorized users.
	"/auth.internals.Internals/*": authdb.AuthServiceAccessGroup,

	// All methods that LUCI Config interacts to perform config validation.
	//
	// Allow all callers as the service itself will check whether the request
	// is from LUCI Config service.
	"/config.Consumer/*": rpcacl.All,
})

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

package projectscope

import (
	"context"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"go.chromium.org/luci/common/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
)

// LookupProjectScopedServiceAccountRPC implements Admin.LookupProjectScopedServiceAccount RPC method.
//
// It assumes authorization has happened already.
type LookupProjectScopedServiceAccountRPC struct {
	// ScopedIdentities stores existing known scoped identities.
	ScopedIdentities func(ctx context.Context) projectscope.ScopedIdentityManager
}

// LookupProjectScopedServiceAccount creates a new project scoped service account.
func (r *LookupProjectScopedServiceAccountRPC) LookupProjectScopedServiceAccount(c context.Context, req *admin.LookupProjectScopedServiceAccountRequest) (*admin.LookupProjectScopedServiceAccountResponse, error) {
	if err := r.validateRequest(c, req); err != nil {
		return nil, err
	}

	// Accept email addresses for convenience
	accountID := req.AccountId
	if strings.Contains(accountID, projectscope.AccountIDDelimiter) {
		accountID = strings.Split(accountID, projectscope.AccountIDDelimiter)[0]
	}

	// Perform the storage lookup on account id
	identity, err := r.ScopedIdentities(c).Lookup(c, accountID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account id not found")
	}
	return &admin.LookupProjectScopedServiceAccountResponse{
		Service: identity.Service,
		Project: identity.Project,
	}, nil
}

// validateRequest performs basic sanity checking on project scope service account lookup requests.
func (r *LookupProjectScopedServiceAccountRPC) validateRequest(c context.Context, req *admin.LookupProjectScopedServiceAccountRequest) error {
	r.logRequest(c, req)

	// Check required proto fields.
	if req.AccountId == "" {
		return status.Errorf(codes.InvalidArgument, "account id must be specified")
	}
	return nil
}

// logRequest logs the body of the request.
func (r *LookupProjectScopedServiceAccountRPC) logRequest(c context.Context, req *admin.LookupProjectScopedServiceAccountRequest) {
	if !logging.IsLogging(c, logging.Debug) {
		return
	}
	m := jsonpb.Marshaler{Indent: " "}
	dump, _ := m.MarshalToString(req)
	logging.Debugf(c, "LookupProjectScopedServiceAccount:\n%s", dump)
}

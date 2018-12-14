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

package serviceaccounts

import (
	"context"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
)

// LookupProjectScopedServiceAccountRPC implements Admin.LookupProjectScopedServiceAccount RPC method.
//
// It assumes authorization has happened already.
type LookupProjectScopedServiceAccountRPC struct {
}

// LookupProjectScopedServiceAccountRPC creates a new project scoped service account.
func (r *LookupProjectScopedServiceAccountRPC) CreateProjectScopedServiceAccount(c context.Context, req *admin.CreateProjectScopedServiceAccountRequest) (*admin.CreateProjectScopedServiceAccountResponse, error) {
	return nil, nil
}

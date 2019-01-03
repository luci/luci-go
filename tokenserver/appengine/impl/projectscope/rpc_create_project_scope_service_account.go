// Copyright 2019 The LUCI Authors.
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
	"fmt"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CreateProjectScopedServiceAccountRPC implements Admin.CreateProjectScopedServiceAccount RPC method.
//
// It assumes authorization has happened already.
type CreateProjectScopedServiceAccountRPC struct {
	// Rules returns project scoped service account rules to use for handling the request
	Rules func(context.Context) (*Rules, error)

	// ScopedIdentities stores existing known scoped identities.
	ScopedIdentities projectscope.ScopedIdentityManager

	// CreateServiceAccount can be set to use in tests
	CreateServiceAccount projectscope.ServiceAccountCreator
}

// CreateProjectScopedServiceAccountRPC creates a new project scoped service account.
func (r *CreateProjectScopedServiceAccountRPC) CreateProjectScopedServiceAccount(c context.Context, req *admin.CreateProjectScopedServiceAccountRequest) (*admin.CreateProjectScopedServiceAccountResponse, error) {
	if err := r.validateRequest(req); err != nil {
		return nil, err
	}

	rules, err := r.Rules(c)
	if err != nil {
		logging.WithError(err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}
	rule, found := rules.rulesPerService[req.Service]
	if !found {
		return nil, status.Errorf(codes.NotFound, "unable to find service config for %s", req.Service)
	}

	// Scoped identity should not exist yet, and we'll try to eagerly create the IAM service account
	scopedIdentity, created, err := r.ScopedIdentities.GetOrCreate(c, req.Service, req.Project, rule.CloudProjectID, r.CreateServiceAccount)
	if err != nil {
		logging.WithError(err)
		return nil, status.Errorf(codes.Internal, err.Error())

	}
	if !created {
		return nil, status.Errorf(codes.AlreadyExists, "the project scoped service account already exists")
	}

	return &admin.CreateProjectScopedServiceAccountResponse{
		AccountId: scopedIdentity.AccountId,
	}, nil
}

// validateRequest performs basic sanity checking on project scope service account creation requests.
func (r *CreateProjectScopedServiceAccountRPC) validateRequest(req *admin.CreateProjectScopedServiceAccountRequest) error {
	if req.Project == "" {
		return fmt.Errorf("project field must be populated")
	}
	if req.Service == "" {
		return fmt.Errorf("service field must be populated")
	}
	return nil
}

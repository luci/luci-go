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
	"fmt"
	"net/http"

	"go.chromium.org/luci/common/gcloud/iam"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ServiceAccountCreator func(identity *projectscope.ScopedIdentity) (bool, error)

// CreateProjectScopedServiceAccountRPC implements Admin.CreateProjectScopedServiceAccount RPC method.
//
// It assumes authorization has happened already.
type CreateProjectScopedServiceAccountRPC struct {
	// ScopedIdentities stores existing known scoped identities.
	ScopedIdentities projectscope.ScopedIdentityManager

	// GcpProjectResolver resembles the service-to-gcp-project mapping.
	//
	// It is used to identify the gcp project for initial service account creation in case
	// of creating a new scoped identity.
	GcpProjectResolver func(service string) string

	// CreateServiceAccount can be set to use in tests
	CreateServiceAccount ServiceAccountCreator
}

// CreateProjectScopedServiceAccountRPC creates a new project scoped service account.
func (r *CreateProjectScopedServiceAccountRPC) CreateProjectScopedServiceAccount(c context.Context, req *admin.CreateProjectScopedServiceAccountRequest) (*admin.CreateProjectScopedServiceAccountResponse, error) {
	if err := r.validateRequest(req); err != nil {
		return nil, err
	}

	var createServiceAccount ServiceAccountCreator
	if r.CreateServiceAccount != nil {
		createServiceAccount = r.CreateServiceAccount
	} else {
		// Create IAM service account, but we'll do it lazily later so creation is not critical.
		createServiceAccount = func(identity *projectscope.ScopedIdentity) (bool, error) {
			asSelf, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(iam.OAuthScope))
			if err != nil {
				return false, nil
			}
			client := &iam.Client{Client: &http.Client{Transport: asSelf}}
			_, err = client.CreateServiceAccount(c, r.GcpProjectResolver(req.Service), identity.AccountId, "")
			if err != nil {
				return false, nil
			}
			return true, nil
		}
	}

	// Scoped identity should not exist yet, and we'll try to eagerly create the IAM service account
	scopedIdentity, created, err := r.ScopedIdentities.GetOrCreate(c, req.Service, req.Project, createServiceAccount)
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

func (r *CreateProjectScopedServiceAccountRPC) validateRequest(req *admin.CreateProjectScopedServiceAccountRequest) error {
	if req.Project == "" {
		return fmt.Errorf("project field must be populated")
	}
	if req.Service == "" {
		return fmt.Errorf("service field must be populated")
	}
	return nil
}

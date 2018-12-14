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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

	// ServiceAccountCreator function is used to create new service accounts.
	//
	// The service accounts represent project scoped identities.
	ServiceAccountCreator auth.ServiceAccountCreator
}

// CreateProjectScopedServiceAccountRPC creates a new project scoped service account.
func (r *CreateProjectScopedServiceAccountRPC) CreateProjectScopedServiceAccount(c context.Context, req *admin.CreateProjectScopedServiceAccountRequest) (*admin.CreateProjectScopedServiceAccountResponse, error) {
	if err := r.validateRequest(req); err != nil {
		return nil, err
	}

	// Check scoped identity storage and create an entry if necessary
	scopedIdentity, err := r.ScopedIdentities.GetOrCreate(c, req.Service, req.Project, nil)
	if err != nil {
		logging.WithError(err)
		if perr, ok := err.(*projectscope.Error); ok {
			switch perr.Reason {
			case projectscope.ErrorAlreadyExists:
				return nil, status.Errorf(codes.AlreadyExists, "service account id already taken")
			}
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	// Actual effect of creating the corresponding service account in the gcp project.
	var gcpProject string
	if r.GcpProjectResolver != nil {
		gcpProject = r.GcpProjectResolver(req.Service)
	} else {
		gcpProject = ""
	}
	created, err := r.ServiceAccountCreator(c, gcpProject, scopedIdentity.AccountId)
	if err != nil {
		logging.Errorf(c, "Error while creating project scoped service account")
		return nil, status.Errorf(codes.Internal, "error while creating service account")
	}
	if !created {
		logging.Errorf(c, "Trying create project scoped service account which already exists")
		return nil, status.Errorf(codes.AlreadyExists, "service account already exists")
	}

	if err != nil {
		return nil, err
	}

	return &admin.CreateProjectScopedServiceAccountResponse{
		AccountId:    scopedIdentity.AccountId,
		GcpProjectId: gcpProject,
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

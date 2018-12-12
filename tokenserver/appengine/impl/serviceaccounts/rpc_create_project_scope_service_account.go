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
	Rules   func(context.Context) (*Rules, error)
	Storage projectscope.ScopedIdentityManager
}

func (r *CreateProjectScopedServiceAccountRPC) CreateProjectScopedServiceAccount(c context.Context, req *admin.CreateProjectScopedServiceAccountRequest) (*admin.CreateProjectScopedServiceAccountResponse, error) {
	accountId, err := r.Storage.GetOrCreateIdentity(c, req.Service, req.Project, req.OverrideAccountId)
	if err != nil {
		logging.WithError(err)
		if perr, ok := err.(*projectscope.Error); ok {
			switch perr.Reason {
			case projectscope.ErrorAlreadyExists:
				return nil, status.Errorf(codes.AlreadyExists, "service account id already exists")
			}
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	gcpProject := "" // TODO(fmatenaar): get gcp project from config
	created, err := auth.GetOrCreateScopedServiceAccount(c, gcpProject, accountId)
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
		AccountId:    accountId,
		GcpProjectId: gcpProject,
	}, nil
}

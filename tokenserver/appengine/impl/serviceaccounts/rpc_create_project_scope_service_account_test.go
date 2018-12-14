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

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCreateProjectScopedServiceAccount(t *testing.T) {
	serviceToProjectMap := map[string]string{
		"svc":  "bar",
		"svc3": "baz",
	}

	serviceAccountMap := map[string]bool{}

	ctx := gaetesting.TestingContext()
	rpc := CreateProjectScopedServiceAccountRPC{
		ScopedIdentities: projectscope.GetScopedIdentities(),
		GcpProjectResolver: func(service string) string {
			return serviceToProjectMap[service]
		},
		ServiceAccountCreator: func(ctx context.Context, project, accountId string) (bool, error) {
			if _, found := serviceAccountMap[accountId]; found {
				return false, nil
			}
			serviceAccountMap[accountId] = true
			return true, nil
		},
	}

	req := &admin.CreateProjectScopedServiceAccountRequest{
		Service: "svc",
		Project: "project",
	}
	generatedAccountId := projectscope.GenerateAccountId(req.Service, req.Project)

	Convey("Happy path", t, func() {

		// Create a first account
		resp, err := rpc.CreateProjectScopedServiceAccount(ctx, req)
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.CreateProjectScopedServiceAccountResponse{
			AccountId:    generatedAccountId,
			GcpProjectId: "bar",
		})

		// Create another account, but redefine req since it won't be used again
		req := &admin.CreateProjectScopedServiceAccountRequest{
			Service: "svc3",
			Project: "project3",
		}
		generatedAccountId := projectscope.GenerateAccountId(req.Service, req.Project)
		resp, err = rpc.CreateProjectScopedServiceAccount(ctx, req)
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.CreateProjectScopedServiceAccountResponse{
			AccountId:    generatedAccountId,
			GcpProjectId: "baz",
		})

	})

	Convey("Account already exists", t, func() {
		_, err := rpc.CreateProjectScopedServiceAccount(ctx, req)
		So(err, ShouldNotBeNil)
		So(err, ShouldResemble, status.Errorf(codes.AlreadyExists, "service account already exists"))
	})

	Convey("Test invalid input", t, func() {
		_, err := rpc.CreateProjectScopedServiceAccount(ctx, &admin.CreateProjectScopedServiceAccountRequest{})
		So(err, ShouldNotBeNil)
	})
}

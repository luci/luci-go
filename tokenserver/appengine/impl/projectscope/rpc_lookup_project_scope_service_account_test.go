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
	"go.chromium.org/luci/common/gcloud/iam"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"

	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLookupProjectScopedServiceAccount(t *testing.T) {
	ctx := gaetesting.TestingContext()
	rpc := CreateProjectScopedServiceAccountRPC{
		ScopedIdentities: projectscope.ScopedIdentities,
		Rules: func(c context.Context) (*Rules, error) {
			return &Rules{
				revision: "fake-revision",
				rulesPerService: map[string]*Rule{
					"svc": {
						CloudProjectID: "bar",
						Rule: &admin.ServiceConfig{
							Service:        "svc",
							CloudProjectId: "bar",
						},
						Revision: "fake-revision",
					},
					"svc3": {
						CloudProjectID: "baz",
						Rule: &admin.ServiceConfig{
							Service:        "svc3",
							CloudProjectId: "baz",
						},
						Revision: "fake-revision",
					},
				},
			}, nil
		},
		CreateServiceAccount: func(ctx context.Context, gcpProject string, identity *projectscope.ScopedIdentity, client projectscope.ServiceAccountClient) (account *iam.ServiceAccount, e error) {
			return &iam.ServiceAccount{}, nil
		},
	}

	lookup := LookupProjectScopedServiceAccountRPC{
		ScopedIdentities: projectscope.ScopedIdentities,
	}

	req := &admin.CreateProjectScopedServiceAccountRequest{
		Service: "svc",
		Project: "project",
	}
	generatedAccountID := projectscope.GenerateAccountID(req.Service, req.Project)

	Convey("Happy path", t, func() {

		// Create a first account
		resp, err := rpc.CreateProjectScopedServiceAccount(ctx, req)
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.CreateProjectScopedServiceAccountResponse{
			AccountEmail: generatedAccountID,
		})

		// Create another account, but redefine req since it won't be used again
		req := &admin.CreateProjectScopedServiceAccountRequest{
			Service: "svc3",
			Project: "project3",
		}
		generatedAccountID := projectscope.GenerateAccountID(req.Service, req.Project)
		resp, err = rpc.CreateProjectScopedServiceAccount(ctx, req)
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.CreateProjectScopedServiceAccountResponse{
			AccountEmail: generatedAccountID,
		})

		lookupReq := &admin.LookupProjectScopedServiceAccountRequest{
			AccountEmail: generatedAccountID,
		}
		lookupResp, err := lookup.LookupProjectScopedServiceAccount(ctx, lookupReq)
		So(err, ShouldBeNil)
		So(lookupResp, ShouldResemble, &admin.LookupProjectScopedServiceAccountResponse{
			Service: req.Service,
			Project: req.Project,
		})

	})

	Convey("Account already exists", t, func() {
		_, err := rpc.CreateProjectScopedServiceAccount(ctx, req)
		So(err, ShouldNotBeNil)
	})

	Convey("Test invalid input", t, func() {
		_, err := rpc.CreateProjectScopedServiceAccount(ctx, &admin.CreateProjectScopedServiceAccountRequest{})
		So(err, ShouldNotBeNil)
	})
}

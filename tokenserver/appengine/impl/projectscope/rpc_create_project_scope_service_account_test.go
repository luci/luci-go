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
	"fmt"
	"math/rand"
	"strconv"

	"go.chromium.org/luci/common/gcloud/iam"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"

	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func newTestGcpProjectResolver() *testGcpProjectResolver {
	return &testGcpProjectResolver{
		m: make(map[string]string),
	}
}

type testGcpProjectResolver struct {
	m map[string]string
}

func (s *testGcpProjectResolver) Resolve(service string) (string, error) {
	if project, ok := s.m[service]; ok {
		return project, nil
	}
	return "", fmt.Errorf("not found")
}

func (s *testGcpProjectResolver) Add(service string, gcpProject string) {
	s.m[service] = gcpProject
}

func (s *testGcpProjectResolver) Remove(service string) {
	delete(s.m, service)
}

func newServiceAccount(luciService string, luciProject string, gcpProject string) *iam.ServiceAccount {
	accountId := projectscope.GenerateAccountId(luciService, luciProject)
	email := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", accountId, luciProject)
	return &iam.ServiceAccount{
		Name:        fmt.Sprintf("projects/%s/serviceAccounts/%s", luciProject, email),
		ProjectId:   gcpProject,
		UniqueId:    strconv.Itoa(rand.Int()),
		Email:       email,
		DisplayName: accountId,
	}
}

func testServiceAccountCreator(ctx context.Context, gcpProject string, identity *projectscope.ScopedIdentity) (*iam.ServiceAccount, bool, error) {
	m := map[string]*iam.ServiceAccount{
		projectscope.GenerateAccountId("svc", "project"):   newServiceAccount("svc", "project", "bar"),
		projectscope.GenerateAccountId("svc3", "project3"): newServiceAccount("svc3", "project3", "baz"),
	}

	fmt.Printf("Looking for identity.AccountId = %s", identity.AccountId)
	if sa, found := m[identity.AccountId]; found {
		return sa, true, nil
	}
	return nil, false, fmt.Errorf("unable to create service account")
}

func TestCreateProjectScopedServiceAccount(t *testing.T) {
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
		CreateServiceAccount: testServiceAccountCreator,
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
			AccountId: generatedAccountId,
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
			AccountId: generatedAccountId,
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

	Convey("", t, func() {
		displayName, err := rpc.generateDisplayName(&admin.CreateProjectScopedServiceAccountRequest{
			Service: "foo",
			Project: "bar",
		})
		So(err, ShouldBeNil)
		So(displayName, ShouldResemble, "{\"luci_svc\":\"foo\",\"luci_prj\":\"bar\"}")
	})
}

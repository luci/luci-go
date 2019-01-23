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
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"

	"go.chromium.org/luci/common/gcloud/iam"

	. "github.com/smartystreets/goconvey/convey"
)

type testServiceAccountClient struct {
	createServiceAccountImpl func(ctx context.Context, gcpProject string, accountId string, displayName string) (*iam.ServiceAccount, error)
}

func (s *testServiceAccountClient) CreateServiceAccount(ctx context.Context, gcpProject string, accountID string, displayName string) (*iam.ServiceAccount, error) {
	return s.createServiceAccountImpl(ctx, gcpProject, accountID, displayName)
}

func TestGenerateDisplayName(t *testing.T) {
	Convey("check invalid input", t, func() {
		_, err := generateDisplayName(&ScopedIdentity{})
		So(err, ShouldNotBeNil)
		_, err = generateDisplayName(&ScopedIdentity{Service: "foo"})
		So(err, ShouldNotBeNil)
	})

	Convey("check shortening to max display name", t, func() {
		identity := &ScopedIdentity{
			AccountID: GenerateAccountID("service", "project"),
			Service:   "serviceaaaaaaaaaaaaaaaaaaaaaaaaaaaaaathisissomereallylongnamethatwillbeshortenedaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Project:   "project",
		}
		generated, err := generateDisplayName(identity)
		So(err, ShouldBeNil)
		So(len(generated), ShouldBeBetweenOrEqual, 0, 100)
		So(generated, ShouldResemble, "{\"luci_svc\":\"serviceaaaaaaaaaaaaaaaaaaaaaaaaaaaaaathisissomereallylongnamethatwillbeshortenedaaaaaaa")
	})

	Convey("check happy path", t, func() {
		identity := &ScopedIdentity{
			AccountID: GenerateAccountID("service", "project"),
			Service:   "service",
			Project:   "project",
		}
		generated, err := generateDisplayName(identity)
		So(err, ShouldBeNil)
		So(generated, ShouldResemble, "{\"luci_svc\":\"service\",\"luci_prj\":\"project\"}")
	})

}

func TestCreateServiceAccount(t *testing.T) {
	Convey("Setup test environment", t, func() {
		ctx := gaetesting.TestingContext()
		service := "service"
		project := "project"
		gcpProject := "gcp-project"

		client := &testServiceAccountClient{
			createServiceAccountImpl: func(ctx context.Context, gcpProject string, accountId string, displayName string) (*iam.ServiceAccount, error) {
				return &iam.ServiceAccount{
					ID:             "1234",
					Name:           accountId,
					ProjectID:      gcpProject,
					UniqueID:       "",
					Email:          "",
					DisplayName:    displayName,
					Oauth2ClientID: "",
				}, nil
			},
		}

		Convey("Test service account field population upon creation", func() {
			identity := &ScopedIdentity{
				AccountID: GenerateAccountID(service, project),
				Service:   service,
				Project:   project,
			}
			displayName, err := generateDisplayName(identity)
			So(err, ShouldBeNil)
			serviceAccount, created, err := CreateServiceAccount(ctx, gcpProject, identity, client)
			So(err, ShouldBeNil)
			So(created, ShouldBeTrue)
			So(serviceAccount, ShouldResemble, &iam.ServiceAccount{
				ID:             "1234",
				Name:           GenerateAccountID(service, project),
				ProjectID:      gcpProject,
				UniqueID:       "",
				Email:          "",
				DisplayName:    displayName,
				Oauth2ClientID: "",
			})
		})

		Convey("createClient returning an error", func() {
			_, created, err := CreateServiceAccount(ctx, "", &ScopedIdentity{}, nil)
			So(err, ShouldNotBeNil)
			So(created, ShouldBeFalse)
		})
	})
}

func TestCreateClient(t *testing.T) {
	Convey("Assert that uninitialized auth library returns error", t, func() {
		_, err := createClient(gaetesting.TestingContext())
		So(err, ShouldNotBeNil)
	})
}

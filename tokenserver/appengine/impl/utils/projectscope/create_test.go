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
	createServiceAccountImpl func(ctx context.Context, gcpProject string, accountID string, displayName string) (*iam.ServiceAccount, error)
	getServiceAccountImpl    func(ctx context.Context, gcpProject, accountID string) (*iam.ServiceAccount, error)
}

func (s *testServiceAccountClient) CreateServiceAccount(ctx context.Context, gcpProject string, accountID string, displayName string) (*iam.ServiceAccount, error) {
	return s.createServiceAccountImpl(ctx, gcpProject, accountID, displayName)
}

func (s *testServiceAccountClient) GetServiceAccount(ctx context.Context, gcpProject string, accountID string) (*iam.ServiceAccount, error) {
	return s.getServiceAccountImpl(ctx, gcpProject, accountID)
}

func TestCreateClient(t *testing.T) {
	Convey("Assert that uninitialized auth library returns error", t, func() {
		_, err := createClient(gaetesting.TestingContext())
		So(err, ShouldNotBeNil)
	})
}

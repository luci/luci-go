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
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/gcloud/iam"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScopedServiceAccountStorage(t *testing.T) {

	createServiceAccount := func(ctx context.Context, gcpProject string, identity *ScopedIdentity, client ServiceAccountClient) (*iam.ServiceAccount, error) {
		return &iam.ServiceAccount{}, nil
	}

	ctx := gaetesting.TestingContext()
	ctx, _ = testclock.UseTime(ctx, time.Date(2000, time.February, 3, 4, 5, 6, 7, time.UTC))

	Convey("Test persistent storage state handling", t, func() {
		storage := &persistentIdentityManager{}

		_, err := storage.Get(ctx, "service1", "project1")
		So(err, ShouldNotBeNil)

		identity, created, err := storage.GetOrCreate(ctx, "service1", "project1", "", createServiceAccount)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, true)
		So(identity.AccountID, ShouldResemble, GenerateAccountID("service1", "project1"))

		identity, err = storage.Get(ctx, "service1", "project1")
		So(err, ShouldBeNil)
		So(identity.AccountID, ShouldResemble, GenerateAccountID("service1", "project1"))

		identity, created, err = storage.GetOrCreate(ctx, "service1", "project1", "", createServiceAccount)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, false)
		So(identity.AccountID, ShouldResemble, GenerateAccountID("service1", "project1"))

		identity, created, err = storage.GetOrCreate(ctx, "service2", "project2", "", createServiceAccount)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, true)
		So(identity.AccountID, ShouldResemble, GenerateAccountID("service2", "project2"))

		identity, created, err = storage.GetOrCreate(ctx, "service2", "project2", "", createServiceAccount)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, false)
		So(identity.AccountID, ShouldResemble, GenerateAccountID("service2", "project2"))
	})

	Convey("Test Lookup functionality", t, func() {
		service := "servicefoo"
		project := "projectbar"
		storage := &persistentIdentityManager{}

		identity, created, err := storage.GetOrCreate(ctx, service, project, "", createServiceAccount)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, true)
		So(identity.AccountID, ShouldResemble, GenerateAccountID(service, project))

		scopedIdentity, err := storage.Lookup(ctx, identity.AccountID)
		So(err, ShouldBeNil)
		So(scopedIdentity, ShouldResemble, &ScopedIdentity{
			AccountID:        identity.AccountID,
			Service:          service,
			Project:          project,
			CreatedTimestamp: clock.Now(ctx).Unix(),
		})
	})

}

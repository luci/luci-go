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
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScopedServiceAccountStorage(t *testing.T) {

	Convey("Test persistent storage state handling", t, func() {
		ctx := gaetesting.TestingContext()
		storage := &persistentIdentityManager{}

		_, err := storage.Get(ctx, "service1", "project1")
		So(err, ShouldNotBeNil)

		identity, created, err := storage.GetOrCreate(ctx, "service1", "project1", "", nil)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, true)
		So(identity.AccountId, ShouldResemble, GenerateAccountId("service1", "project1"))

		identity, err = storage.Get(ctx, "service1", "project1")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, GenerateAccountId("service1", "project1"))

		identity, created, err = storage.GetOrCreate(ctx, "service1", "project1", "", nil)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, false)
		So(identity.AccountId, ShouldResemble, GenerateAccountId("service1", "project1"))

		identity, created, err = storage.GetOrCreate(ctx, "service2", "project2", "", nil)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, true)
		So(identity.AccountId, ShouldResemble, GenerateAccountId("service2", "project2"))

		identity, created, err = storage.GetOrCreate(ctx, "service2", "project2", "", nil)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, false)
		So(identity.AccountId, ShouldResemble, GenerateAccountId("service2", "project2"))
	})

	Convey("Test Lookup functionality", t, func() {
		service := "servicefoo"
		project := "projectbar"
		ctx := gaetesting.TestingContext()
		storage := &persistentIdentityManager{}

		identity, created, err := storage.GetOrCreate(ctx, service, project, "", nil)
		So(err, ShouldBeNil)
		So(created, ShouldResemble, true)
		So(identity.AccountId, ShouldResemble, GenerateAccountId(service, project))

		scopedIdentity, err := storage.Lookup(ctx, identity.AccountId)
		So(err, ShouldBeNil)
		So(scopedIdentity, ShouldResemble, &ScopedIdentity{
			AccountId: identity.AccountId,
			Service:   service,
			Project:   project,
		})
	})

}

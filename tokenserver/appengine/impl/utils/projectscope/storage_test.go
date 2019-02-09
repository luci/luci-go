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
	"go.chromium.org/luci/common/gcloud/iam"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScopedServiceAccountStorage(t *testing.T) {

	Convey("Test successful creation of several identities", t, func() {
		ctx := gaetesting.TestingContext()
		var err error
		var actual *ProjectIdentity
		var expected *ProjectIdentity

		storage := &persistentIdentityManager{}

		expected = &ProjectIdentity{
			Email:          "sa1@project1.iamserviceaccounts.com",
			Project:        "project1",
			ServiceAccount: iam.ServiceAccount{},
		}

		actual, err = storage.Create(ctx, expected.Project, expected.Email, expected.ServiceAccount)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, expected)

		expected = &ProjectIdentity{
			Email:          "sa1@project2.iamserviceaccounts.com",
			Project:        "project2",
			ServiceAccount: iam.ServiceAccount{},
		}

		actual, err = storage.Create(ctx, expected.Project, expected.Email, expected.ServiceAccount)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, expected)

		expected = &ProjectIdentity{
			Email:          "sa3@project3.iamserviceaccounts.com",
			Project:        "project3",
			ServiceAccount: iam.ServiceAccount{},
		}

		actual, err = storage.Create(ctx, expected.Project, expected.Email, expected.ServiceAccount)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, expected)

		Convey("Test error raised upon duplicate", func() {
			_, err = storage.Create(ctx, expected.Project, expected.Email, expected.ServiceAccount)
			So(err, ShouldNotBeNil)
			So(err, ShouldResemble, ErrAlreadyExists)
		})

		Convey("Test reading by identities by project and email", func() {
			actual, err = storage.Lookup(ctx, expected)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, expected)

			actual, err = storage.LookupByProject(ctx, expected.Project)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, expected)
		})

		Convey("Test deleting identity", func() {
			actual, err = storage.Lookup(ctx, expected)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, expected)

			err = storage.Delete(ctx, expected)
			So(err, ShouldBeNil)

			_, err = storage.Lookup(ctx, expected)
			So(err, ShouldNotBeNil)

		})
	})
}

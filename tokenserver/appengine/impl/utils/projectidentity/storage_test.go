// Copyright 2019 The LUCI Authors.
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

package projectidentity

import (
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

		storage := &persistentStorage{}

		expected = &ProjectIdentity{
			Email:   "sa1@project1.iamserviceaccounts.com",
			Project: "project1",
		}

		actual, err = storage.Create(ctx, expected)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, expected)

		expected = &ProjectIdentity{
			Email:   "sa1@project2.iamserviceaccounts.com",
			Project: "project2",
		}

		actual, err = storage.Create(ctx, expected)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, expected)

		expected = &ProjectIdentity{
			Email:   "sa3@project3.iamserviceaccounts.com",
			Project: "project3",
		}

		actual, err = storage.Create(ctx, expected)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, expected)

		Convey("Test error raised upon duplicate", func() {
			actual, err = storage.Create(ctx, expected)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, expected)
		})

		Convey("Test reading by identities by project and email", func() {
			actual, err = storage.lookup(ctx, expected)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, expected)

			actual, err = storage.LookupByProject(ctx, expected.Project)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, expected)
		})

		Convey("Test deleting identity", func() {
			actual, err = storage.lookup(ctx, expected)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, expected)

			err = storage.Delete(ctx, expected)
			So(err, ShouldBeNil)

			_, err = storage.lookup(ctx, expected)
			So(err, ShouldNotBeNil)

		})

		Convey("Test update identity", func() {
			// Make sure we successfully read the expected entry
			actual, err := storage.LookupByProject(ctx, expected.Project)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, expected)

			// Create an updated entry
			updated := &ProjectIdentity{
				Project: expected.Project,
				Email:   "foo@bar.com",
			}
			So(updated, ShouldNotResemble, expected)

			// Update the entry in the datastore
			actual, err = storage.Update(ctx, updated)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, updated)

			// Read from datastore, verify its changed
			actual, err = storage.lookup(ctx, expected)
			So(err, ShouldBeNil)
			So(updated, ShouldNotResemble, expected)
			So(actual, ShouldNotResemble, expected)
			So(actual, ShouldResemble, updated)

			// Create a new entry via update
			expected = &ProjectIdentity{
				Project: "project4",
				Email:   "foo@example.com",
			}
			actual, err = storage.Update(ctx, expected)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, expected)

		})
	})
}

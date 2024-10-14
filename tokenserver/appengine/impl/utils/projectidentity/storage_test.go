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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestScopedServiceAccountStorage(t *testing.T) {

	ftt.Run("Test successful creation of several identities", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(expected))

		expected = &ProjectIdentity{
			Email:   "sa1@project2.iamserviceaccounts.com",
			Project: "project2",
		}

		actual, err = storage.Create(ctx, expected)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(expected))

		expected = &ProjectIdentity{
			Email:   "sa3@project3.iamserviceaccounts.com",
			Project: "project3",
		}

		actual, err = storage.Create(ctx, expected)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(expected))

		t.Run("Test error raised upon duplicate", func(t *ftt.Test) {
			actual, err = storage.Create(ctx, expected)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Resemble(expected))
		})

		t.Run("Test reading by identities by project and email", func(t *ftt.Test) {
			actual, err = storage.lookup(ctx, expected)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Resemble(expected))

			actual, err = storage.LookupByProject(ctx, expected.Project)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Resemble(expected))
		})

		t.Run("Test deleting identity", func(t *ftt.Test) {
			actual, err = storage.lookup(ctx, expected)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Resemble(expected))

			err = storage.Delete(ctx, expected)
			assert.Loosely(t, err, should.BeNil)

			_, err = storage.lookup(ctx, expected)
			assert.Loosely(t, err, should.NotBeNil)

		})

		t.Run("Test update identity", func(t *ftt.Test) {
			// Make sure we successfully read the expected entry
			actual, err := storage.LookupByProject(ctx, expected.Project)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Resemble(expected))

			// Create an updated entry
			updated := &ProjectIdentity{
				Project: expected.Project,
				Email:   "foo@bar.com",
			}
			assert.Loosely(t, updated, should.NotResemble(expected))

			// Update the entry in the datastore
			actual, err = storage.Update(ctx, updated)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Resemble(updated))

			// Read from datastore, verify its changed
			actual, err = storage.lookup(ctx, expected)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updated, should.NotResemble(expected))
			assert.Loosely(t, actual, should.NotResemble(expected))
			assert.Loosely(t, actual, should.Resemble(updated))

			// Create a new entry via update
			expected = &ProjectIdentity{
				Project: "project4",
				Email:   "foo@example.com",
			}
			actual, err = storage.Update(ctx, expected)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Resemble(expected))

		})
	})
}

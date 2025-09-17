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
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestScopedServiceAccountStorage(t *testing.T) {
	storage := &persistentStorage{}
	ctx := gaetesting.TestingContext()

	makeIdent := func(project, email string) *ProjectIdentity {
		return &ProjectIdentity{Project: project, Email: email}
	}

	// Nothing in the storage.
	_, err := storage.LookupByProject(ctx, "p")
	assert.That(t, err, should.Equal(ErrNotFound))

	// Update can create an entry.
	assert.NoErr(t, storage.Update(ctx, makeIdent("p", "e1")))
	ident, err := storage.LookupByProject(ctx, "p")
	assert.NoErr(t, err)
	assert.That(t, ident.Email, should.Equal("e1"))

	// Update can update an entry.
	assert.NoErr(t, storage.Update(ctx, makeIdent("p", "e2")))
	ident, err = storage.LookupByProject(ctx, "p")
	assert.NoErr(t, err)
	assert.That(t, ident.Email, should.Equal("e2"))

	// Noop update is also fine.
	assert.NoErr(t, storage.Update(ctx, makeIdent("p", "e2")))
	ident, err = storage.LookupByProject(ctx, "p")
	assert.NoErr(t, err)
	assert.That(t, ident.Email, should.Equal("e2"))

	// Delete deletes the entry.
	assert.NoErr(t, storage.Delete(ctx, "p"))
	_, err = storage.LookupByProject(ctx, "p")
	assert.That(t, err, should.Equal(ErrNotFound))

	// Deleting a non-existing entry is fine.
	assert.NoErr(t, storage.Delete(ctx, "p"))
}

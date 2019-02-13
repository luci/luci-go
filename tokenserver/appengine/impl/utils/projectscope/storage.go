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

package projectscope

import (
	"context"
	"errors"

	"go.chromium.org/luci/common/retry/transient"

	ds "go.chromium.org/gae/service/datastore"
)

var (
	// ErrNotFound indicates that the entity which was queried does not exist in the storage.
	ErrNotFound = errors.New("not found")
)

// projectIdentities is the default storage for all scoped identities.
var projectIdentities = &persistentIdentityManager{}

// ProjectIdentities returns the global scoped identity storage.
func ProjectIdentities(ctx context.Context) ProjectIdentityManager {
	return projectIdentities
}

// ProjectIdentityManager interface declares the interface to the scoped identity storage.
type ProjectIdentityManager interface {

	// Create an identity or update if it already exists.
	Create(c context.Context, identity *ProjectIdentity) (*ProjectIdentity, error)

	// Update an identity in the storage.
	Update(c context.Context, identity *ProjectIdentity) (*ProjectIdentity, error)

	// Delete an identity from the storage.
	Delete(c context.Context, identity *ProjectIdentity) error

	// LookupByProject performs a lookup by project name.
	LookupByProject(c context.Context, project string) (*ProjectIdentity, error)
}

// ProjectIdentity defines a scoped identity in the storage.
type ProjectIdentity struct {
	_kind   string `gae:"$kind,ScopedIdentity"`
	Project string `gae:"$id"`
	Email   string
}

// persistentIdentityManager implements ScopedIdentityManager.
type persistentIdentityManager struct {
}

// lookup reads an identity from the storage based on what fields are set in the identity struct.
func (s *persistentIdentityManager) lookup(c context.Context, identity *ProjectIdentity) (*ProjectIdentity, error) {
	tmp := *identity
	if err := ds.Get(c, &tmp); err != nil {
		switch {
		case err == ds.ErrNoSuchEntity:
			return nil, ErrNotFound
		case err != nil:
			return nil, transient.Tag.Apply(err)
		}
	}
	return &tmp, nil
}

// LookupByProject returns the project identity stored for a given project.
func (s *persistentIdentityManager) LookupByProject(c context.Context, project string) (*ProjectIdentity, error) {
	return s.lookup(c, &ProjectIdentity{Project: project})
}

// Delete removes an identity from the storage.
func (s *persistentIdentityManager) Delete(c context.Context, identity *ProjectIdentity) error {
	return ds.Delete(c, identity)
}

// Create stores a new entry for a project identity.
func (s *persistentIdentityManager) Create(c context.Context, identity *ProjectIdentity) (*ProjectIdentity, error) {
	return s.Update(c, identity)
}

// Update allows an identity to be updated, e.g. when the service account email changes.
func (s *persistentIdentityManager) Update(c context.Context, identity *ProjectIdentity) (*ProjectIdentity, error) {
	tmp, err := s.lookup(c, identity)
	switch {
	case err == nil && *tmp == *identity: // Doesn't need update
		return identity, nil
	case err != nil && err != ErrNotFound: // Lookup error to propagate
		return nil, err
	}

	if err := ds.Put(c, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

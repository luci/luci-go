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
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
)

// ErrNotFound indicates that the entity which was queried does not exist in
// the storage.
var ErrNotFound = errors.New("not found")

// projectIdentities is the default storage for all scoped identities.
var projectIdentities = &persistentStorage{}

// ProjectIdentities returns the global scoped identity storage.
func ProjectIdentities(_ context.Context) Storage {
	return projectIdentities
}

// Storage interface declares methods for the scoped identity storage.
type Storage interface {
	// Update updates an identity in the storage, creating it if necessary.
	Update(ctx context.Context, ident *ProjectIdentity) error
	// Delete removes an identity from the storage if it is there.
	Delete(ctx context.Context, project string) error
	// LookupByProject performs an identity lookup by project name.
	LookupByProject(ctx context.Context, project string) (*ProjectIdentity, error)
}

// ProjectIdentity defines a scoped identity in the storage.
type ProjectIdentity struct {
	_kind   string `gae:"$kind,ScopedIdentity"`
	Project string `gae:"$id"`
	Email   string `gae:",noindex"`
}

// persistentStorage implements Storage.
type persistentStorage struct{}

func lookup(ctx context.Context, project string) (*ProjectIdentity, error) {
	ident := &ProjectIdentity{Project: project}
	switch err := datastore.Get(ctx, ident); {
	case err == datastore.ErrNoSuchEntity:
		return nil, ErrNotFound
	case err != nil:
		return nil, transient.Tag.Apply(err)
	default:
		return ident, nil
	}
}

// LookupByProject returns the project identity stored for a given project.
func (s *persistentStorage) LookupByProject(ctx context.Context, project string) (*ProjectIdentity, error) {
	// Note: here and elsewhere we make sure to avoid accidentally reusing
	// callers datastore transaction, since our usage of the datastore in the
	// Storage interface implementation is an internal detail. There's no promise
	// of supporting datastore transactions in the Storage interface contract.
	return lookup(datastore.WithoutTransaction(ctx), project)
}

// Delete removes an identity from the storage.
func (s *persistentStorage) Delete(ctx context.Context, project string) error {
	logging.Debugf(ctx, "Deleting project scoped identity for %q", project)
	return transient.Tag.Apply(datastore.Delete(datastore.WithoutTransaction(ctx), &ProjectIdentity{
		Project: project,
	}))
}

// Update updates an identity in the storage, creating it if necessary.
func (s *persistentStorage) Update(ctx context.Context, identity *ProjectIdentity) error {
	return transient.Tag.Apply(datastore.RunInTransaction(datastore.WithoutTransaction(ctx), func(ctx context.Context) error {
		switch cur, err := lookup(ctx, identity.Project); {
		case err != nil && err != ErrNotFound:
			return err
		case err == nil && cur.Email == identity.Email:
			return nil // already up to date
		default:
			if cur == nil {
				logging.Infof(ctx, "Creating project scoped identity for %q (email = %q)", identity.Project, identity.Email)
			} else {
				logging.Infof(ctx, "Project scoped identity of %q has changed: %q => %q", identity.Project, cur.Email, identity.Email)
			}
			return datastore.Put(ctx, identity)
		}
	}, nil))
}

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
	"errors"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/common/retry/transient"

	ds "go.chromium.org/gae/service/datastore"
)

var (
	// ErrAlreadyExists indicated that the entity already exists in the storage.
	ErrAlreadyExists = errors.New("already exists")

	// ErrNotFound indicates that the entity which was queried does not exist in the storage.
	ErrNotFound = errors.New("not found")
)

// ScopedIdentities is the default storage for all scoped identities.
var scopedIdentities = &persistentIdentityManager{}

// ScopedIdentities returns the global scoped identity storage.
func ScopedIdentities(ctx context.Context) ScopedIdentityManager {
	return scopedIdentities
}

// ScopedIdentityManager interface declares the interface to the scoped identity storage.
type ScopedIdentityManager interface {
	// GetOrCreate retrieves a scoped identity from the storage if it exists, or otherwise creates it first.
	GetOrCreate(c context.Context, service, project, gcpProject string, onCreate ServiceAccountCreator) (*ScopedIdentity, bool, error)

	// Get retrieves a scoped identity from the storage, keyed by (service, project) pair.
	Get(c context.Context, service, project string) (*ScopedIdentity, error)

	// Lookup performs an identity lookup by account id.
	Lookup(c context.Context, accountID string) (*ScopedIdentity, error)
}

// ScopedIdentity defines a scoped identity in the storage.
type ScopedIdentity struct {
	_kind            string `gae:"$kind,ScopedIdentity"`
	AccountID        string `gae:"$id"`
	Service          string
	Project          string
	GcpProject       string
	CreatedTimestamp int64
	ServiceAccount   iam.ServiceAccount `gae:",noindex"`
}

// NewScopedIdentity creates a new scoped identity.
func NewScopedIdentity(service, project, gcpProject string) *ScopedIdentity {
	identity := &ScopedIdentity{
		Service:          service,
		Project:          project,
		GcpProject:       gcpProject,
		CreatedTimestamp: 0,
	}

	identity.AccountID = GenerateAccountID(service, project)
	return identity
}

// persistentIdentityManager implements ScopedIdentityManager.
type persistentIdentityManager struct {
}

func (s *persistentIdentityManager) getInternal(c context.Context, identity *ScopedIdentity) error {
	if err := ds.Get(c, identity); err != nil {
		switch {
		case err == ds.ErrNoSuchEntity:
			return ErrNotFound
		case err != nil:
			return transient.Tag.Apply(err)
		}
	}
	return nil
}

func (s *persistentIdentityManager) Lookup(c context.Context, accountEmail string) (*ScopedIdentity, error) {
	identity := &ScopedIdentity{
		AccountID: accountEmail,
	}
	if err := s.getInternal(c, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func (s *persistentIdentityManager) Get(c context.Context, service, project string) (*ScopedIdentity, error) {
	identity := NewScopedIdentity(service, project, "")
	if err := s.getInternal(c, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func (s *persistentIdentityManager) GetOrCreate(c context.Context, service, project, gcpProject string, createServiceAccount ServiceAccountCreator) (*ScopedIdentity, bool, error) {
	identity := NewScopedIdentity(service, project, gcpProject)

	err := s.getInternal(c, identity)
	if err == nil {
		return identity, false, nil
	}

	// If err is anything other than ErrNotFound, emit transient error
	if err != ErrNotFound {
		return nil, false, err
	}

	// If this function is not given a service account creator function
	// then we are done here.
	if createServiceAccount == nil {
		return nil, false, err
	}

	// Attempt to create service account.
	// Will fetch the service account if it was already created.
	serviceAccount, err := createServiceAccount(c, gcpProject, identity, nil)
	if err != nil {
		if err == ErrAlreadyExists {
			return nil, false, transient.Tag.Apply(ErrAlreadyExists)
		}
		return nil, false, err
	}

	identity.CreatedTimestamp = clock.Now(c).Unix()
	identity.ServiceAccount = *serviceAccount

	// Using transaction to leverage retry logic
	err = ds.RunInTransaction(c, func(c context.Context) error {
		return ds.Put(c, identity)
	}, nil)
	if err != nil {
		return nil, false, err
	}
	return identity, true, nil
}

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

	"go.chromium.org/luci/common/gcloud/iam"

	ds "go.chromium.org/gae/service/datastore"
)

const (
	// ErrorAlreadyExists indicated that the entity already exists in the storage.
	ErrorAlreadyExists = iota

	// ErrorNotFound indicated that the entity which was queried does not exist in the storage.
	ErrorNotFound = iota
)

// Error represents a storage error.
type Error struct {
	Reason int
}

func (e *Error) Error() string {
	switch e.Reason {
	case ErrorAlreadyExists:
		return "Entry already exists"
	}
	return "Storage error"
}

// ScopedIdentities is the default storage for all scoped identities.
var scopedIdentities = newScopedIdentities()

// ScopedIdentities returns the global scoped identity storage.
func ScopedIdentities(ctx context.Context) ScopedIdentityManager {
	return scopedIdentities
}

// newScopedIdentities creates a new identity storage.
func newScopedIdentities() ScopedIdentityManager {
	return &persistentIdentityManager{}
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
	_kind          string `gae:"$kind,ScopedIdentity"`
	AccountID      string `gae:"$id"`
	Service        string
	Project        string
	GcpProject     string
	Created        bool
	ServiceAccount iam.ServiceAccount `gae:",noindex"`
}

// NewScopedIdentity creates a new scoped identity.
func NewScopedIdentity(service, project, gcpProject string) *ScopedIdentity {
	identity := &ScopedIdentity{
		Service:    service,
		Project:    project,
		GcpProject: gcpProject,
		Created:    false,
	}

	identity.AccountID = GenerateAccountID(service, project)
	return identity
}

// persistentIdentityManager implements ScopedIdentityManager.
type persistentIdentityManager struct {
}

func (s *persistentIdentityManager) Lookup(c context.Context, accountID string) (*ScopedIdentity, error) {
	identity := &ScopedIdentity{
		AccountID: accountID,
	}
	if err := ds.Get(c, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func (s *persistentIdentityManager) Get(c context.Context, service, project string) (*ScopedIdentity, error) {
	identity := NewScopedIdentity(service, project, "")
	if err := ds.Get(c, identity); err != nil {
		return nil, &Error{Reason: ErrorNotFound}
	}
	return identity, nil
}

func (s *persistentIdentityManager) GetOrCreate(c context.Context, service, project, gcpProject string, onCreate ServiceAccountCreator) (*ScopedIdentity, bool, error) {
	identity := NewScopedIdentity(service, project, gcpProject)
	var entryExists bool

	// Transactionally get-or-create new identity entry
	err := ds.RunInTransaction(c,
		func(c context.Context) error {
			// Check whether we already have an entry in the datastore
			err := ds.Get(c, identity)
			entryExists = err == nil

			// If the identity doesn't indicate it has been created, give callback the opportunity
			// to create the service account.
			if !identity.Created && onCreate != nil {
				serviceAccount, created, err := onCreate(c, gcpProject, identity, nil)
				if err != nil {
					return err
				}
				identity.Created = created
				identity.ServiceAccount = *serviceAccount
			}

			// Write to datastore if identity didn't previously exist
			if !entryExists {
				if err = ds.Put(c, identity); err != nil {
					return err
				}
			}
			return nil
		},
		&ds.TransactionOptions{XG: false, Attempts: 5, ReadOnly: false})
	if err != nil {
		return nil, false, err
	}

	// Assertion
	if identity.Service != service {
		panic("identity.Service != service while reading storage, this should never happen")
	}
	if identity.Project != project {
		panic("identity.Project != project while reading storage, this should never happen")
	}
	return identity, !entryExists, nil
}

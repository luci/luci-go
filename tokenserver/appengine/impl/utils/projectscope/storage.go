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
	ErrorAlreadyExists = iota
	ErrorNotFound      = iota
)

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

var ScopedIdentities = NewScopedIdentities()

func NewScopedIdentities() ScopedIdentityManager {
	return &persistentIdentityManager{}
}

type ScopedIdentityManager interface {
	GetOrCreate(c context.Context, service, project, gcpProject string, onCreate ServiceAccountCreator) (*ScopedIdentity, bool, error)
	Get(c context.Context, service, project string) (*ScopedIdentity, error)
	Lookup(c context.Context, accountId string) (*ScopedIdentity, error)
}

type ScopedIdentity struct {
	_kind          string `gae:"$kind,ScopedIdentity"`
	AccountId      string `gae:"$id"`
	Service        string
	Project        string
	GcpProject     string
	Created        bool
	ServiceAccount iam.ServiceAccount
}

// NewScopedIdentity creates a new scoped identity.
func NewScopedIdentity(service, project, gcpProject string) *ScopedIdentity {
	identity := &ScopedIdentity{
		Service:    service,
		Project:    project,
		GcpProject: gcpProject,
		Created:    false,
	}

	identity.AccountId = GenerateAccountId(service, project)
	return identity
}

type persistentIdentityManager struct {
}

func (s *persistentIdentityManager) Lookup(c context.Context, accountId string) (*ScopedIdentity, error) {
	identity := &ScopedIdentity{
		AccountId: accountId,
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
	ds.RunInTransaction(c,
		func(c context.Context) error {
			// Check whether we already have an entry in the datastore
			err := ds.Get(c, identity)
			entryExists = err == nil

			// If the identity doesn't indicate it has been created, give callback the opportunity
			// to create the service account.
			if !identity.Created && onCreate != nil {
				serviceAccount, created, err := onCreate(c, gcpProject, identity)
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

	// Assertion
	if identity.Service != service || identity.Project != project {
		panic("this should never happen")
	}
	return identity, !entryExists, nil
}

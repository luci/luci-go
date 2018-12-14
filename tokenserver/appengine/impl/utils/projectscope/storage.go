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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

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

func GetScopedIdentities() ScopedIdentityManager {
	return scopedIdentityManager
}

var scopedIdentityManager ScopedIdentityManager

func init() {
	scopedIdentityManager = &persistentIdentityManager{}
}

type ScopedIdentityManager interface {
	GetOrCreate(c context.Context, service, project string, onCreate func(identity *ScopedIdentity) (bool, error)) (*ScopedIdentity, error)
	Get(c context.Context, service, project string) (*ScopedIdentity, error)
}

type ScopedIdentity struct {
	_kind     string `gae:"$kind,ScopedIdentity"`
	Id        string `gae:"$id"`
	AccountId string
	Service   string
	Project   string
	Created   bool
}

// NewScopedIdentity creates a new scoped identity.
func NewScopedIdentity(service, project string) *ScopedIdentity {
	identity := &ScopedIdentity{
		Service: service,
		Project: project,
		Created: false,
	}

	identity.AccountId = GenerateAccountId(service, project)
	_, _ = identity.calculateKey()
	return identity
}

func (s *ScopedIdentity) calculateKey() (string, error) {
	if s.Service == "" || s.Project == "" {
		return "", fmt.Errorf("unable to calculate key over empty fields")
	}
	s.Id = calculateKey(s.Service, s.Project)
	return s.Id, nil
}

func calculateKey(project, service string) string {
	structure := struct {
		Service string `json:"service"`
		Project string `json:"project"`
	}{
		Service: service,
		Project: project,
	}

	bytes, err := json.Marshal(&structure)
	if err != nil {
		panic("marshalling should never fail")
	}
	hash := sha256.Sum256(bytes)
	slice := hash[:]
	return hex.EncodeToString(slice)
}

type persistentIdentityManager struct {
}

func (s *persistentIdentityManager) Get(c context.Context, service, project string) (*ScopedIdentity, error) {
	identity := NewScopedIdentity(service, project)
	if err := ds.Get(c, identity); err != nil {
		return nil, &Error{Reason: ErrorNotFound}
	}
	return identity, nil
}

func (s *persistentIdentityManager) GetOrCreate(c context.Context, service, project string, onCreate func(identity *ScopedIdentity) (bool, error)) (*ScopedIdentity, error) {
	identity := NewScopedIdentity(service, project)

	// Transactionally get-or-create new identity entry
	ds.RunInTransaction(c,
		func(c context.Context) error {
			// Check whether we already have an entry in the datastore
			err := ds.Get(c, identity)
			entryExists := err == nil

			// If the identity doesn't indicate it has been created, give callback the opportunity
			// to create the service account.
			if !identity.Created && onCreate != nil {
				created, err := onCreate(identity)
				if err != nil {
					return err
				}
				identity.Created = created
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
	return identity, nil
}

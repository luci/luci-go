// Copyright 2025 The LUCI Authors.
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

// Package errors contains errors specific to the administration and
// distribution of the AuthDB.
package errors

import (
	stderrors "errors"

	"go.chromium.org/luci/common/errors"
)

var (
	//////////// Errors related to accessing or modifying the AuthDB. ////////////

	// ErrAlreadyExists is returned when an entity already exists.
	ErrAlreadyExists = stderrors.New("entity already exists")

	// ErrPermissionDenied is returned when the user does not have permission for
	// the requested entity operation.
	ErrPermissionDenied = stderrors.New("permission denied")

	// ErrInvalidArgument is a generic error returned when an argument is invalid.
	// It may be wrapped with more specific details on the invalidity.
	ErrInvalidArgument = stderrors.New("invalid argument")

	// ErrInvalidName is returned when a supplied entity name is invalid.
	ErrInvalidName = stderrors.New("invalid entity name")

	// ErrInvalidReference is returned when a referenced entity name is invalid.
	ErrInvalidReference = stderrors.New("some referenced groups don't exist")

	// ErrInvalidIdentity is returned when a referenced identity or glob is
	// invalid.
	ErrInvalidIdentity = stderrors.New("invalid identity")

	// ErrConcurrentModification is returned when an entity is modified by two
	// concurrent operations.
	ErrConcurrentModification = stderrors.New("concurrent modification")

	// ErrReferencedEntity is returned when an entity cannot be deleted because
	// it is referenced elsewhere.
	ErrReferencedEntity = stderrors.New("cannot delete referenced entity")
	// ErrCyclicDependency is returned when an update would create a cyclic
	// dependency.
	ErrCyclicDependency = stderrors.New("groups can't have cyclic dependencies")

	///////////// Errors related to using a snapshot of the AuthDB. //////////////

	// ErrAuthDBMissingRealms is returned when an AuthDB is missing the Realms
	// field.
	ErrAuthDBMissingRealms = errors.New("AuthDB missing Realms field")

	// ErrSnapshotMissingAuthDB is returned when an AuthDBSnapshot is missing the
	// AuthDB field.
	ErrSnapshotMissingAuthDB = errors.New("AuthDBSnapshot missing AuthDB field")
)

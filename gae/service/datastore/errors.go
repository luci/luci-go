// Copyright 2015 The LUCI Authors.
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

package datastore

import (
	"fmt"
	"reflect"

	"google.golang.org/appengine/datastore"

	"go.chromium.org/luci/common/errors"
)

type stopErr struct{}

func (stopErr) Error() string { return "stop iteration" }

type limitExceeded struct{}

func (limitExceeded) Error() string { return "limit exceeded" }

// These errors are returned by various datastore.Interface methods.
var (
	ErrNoSuchEntity          = datastore.ErrNoSuchEntity
	ErrConcurrentTransaction = datastore.ErrConcurrentTransaction
	ErrInvalidKey            = datastore.ErrInvalidKey

	// Stop is understood by various services to stop iterative processes. Examples
	// include datastore.Interface.Run's callback.
	Stop = stopErr{}

	// ErrLimitExceeded is used to indicate the iteration limit has been exceeded.
	ErrLimitExceeded = limitExceeded{}
)

// IsErrNoSuchEntity tests if an error is ErrNoSuchEntity,
// or is a MultiError that contains ErrNoSuchEntity and no other errors.
func IsErrNoSuchEntity(err error) (found bool) {
	errors.WalkLeaves(err, func(ierr error) bool {
		found = ierr == ErrNoSuchEntity
		// If we found an ErrNoSuchEntity, continue walking.
		// If we found a different type of error, signal WalkLeaves to stop walking by returning false.
		// WalkLeaves does not walk nil errors nor wrapper errors.
		return found
	})
	return
}

// ErrFieldMismatch is returned when a field is to be loaded into a different
// type than the one it was stored from, or when a field is missing or
// unexported in the destination struct.
// StructType is the type of the struct pointed to by the destination argument
// passed to Get or to Iterator.Next.
type ErrFieldMismatch struct {
	StructType reflect.Type
	FieldName  string
	Reason     string
}

func (e *ErrFieldMismatch) Error() string {
	return fmt.Sprintf("gae: cannot load field %q into a %q: %s",
		e.FieldName, e.StructType, e.Reason)
}

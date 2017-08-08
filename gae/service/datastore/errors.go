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

	"go.chromium.org/gae"

	"go.chromium.org/luci/common/errors"

	"google.golang.org/appengine/datastore"
)

// These errors are returned by various datastore.Interface methods.
var (
	ErrNoSuchEntity          = datastore.ErrNoSuchEntity
	ErrConcurrentTransaction = datastore.ErrConcurrentTransaction

	// Stop is an alias for "go.chromium.org/gae".Stop
	Stop = gae.Stop
)

// MakeErrInvalidKey returns an errors.Annotator instance that wraps an invalid
// key error. Calling IsErrInvalidKey on this Annotator or its derivatives will
// return true.
func MakeErrInvalidKey(reason string, args ...interface{}) *errors.Annotator {
	return errors.Annotate(datastore.ErrInvalidKey, reason, args...)
}

// IsErrInvalidKey tests if a given error is a wrapped datastore.ErrInvalidKey
// error.
func IsErrInvalidKey(err error) bool { return errors.Unwrap(err) == datastore.ErrInvalidKey }

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

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package commonErrors

import (
	"reflect"

	"appengine/datastore"
	"appengine/memcache"
	"appengine/taskqueue"
)

// Common errors originating from the GAE SDK
var (
	ErrCASConflictMC = memcache.ErrCASConflict
	ErrNoStatsMC     = memcache.ErrNoStats
	ErrCacheMissMC   = memcache.ErrCacheMiss
	ErrNotStoredMC   = memcache.ErrNotStored
	ErrServerErrorMC = memcache.ErrServerError

	ErrInvalidKeyDS            = datastore.ErrInvalidKey
	ErrConcurrentTransactionDS = datastore.ErrConcurrentTransaction
	ErrNoSuchEntityDS          = datastore.ErrNoSuchEntity
	ErrInvalidEntityTypeDS     = datastore.ErrInvalidEntityType

	ErrTaskAlreadyAddedTQ = taskqueue.ErrTaskAlreadyAdded
)

// TODO(riannucci): Add common internal errors which originate from the
// dev_appserver SDK

// ErrFieldMismatchDS returns a datastore.ErrFieldMismatch struct filled with
// the given specification
func ErrFieldMismatchDS(structType reflect.Type, fieldName, reason string) error {
	return &datastore.ErrFieldMismatch{
		StructType: structType,
		FieldName:  fieldName,
		Reason:     reason,
	}
}

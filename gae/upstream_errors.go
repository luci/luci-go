// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file contains types and values which are mirrors/duplicates of the
// upstream SDK errors. This exists so that users can depend solely on this
// wrapper library without also needing to import the SDK.
//
// Format of the errors is Err<sub>Name, where <sub> is one of the 2/3-letter
// All Caps subpackage codes (e.g. DS, TQ, MC, etc.). Name is the same as the
// original SDK error.
//
// Note that this only replicates the 'named' errors (which a user might compare
// for equality or observe by-type in their code like ErrDSNoSuchEntity). The
// underlying implementations can (and do) return many more sorts of non-named
// and custom errors.

package gae

import (
	"fmt"
	"reflect"
	"sync"

	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/memcache"
	"google.golang.org/appengine/taskqueue"
)

// These are pass-through versions from the managed-VM SDK. All implementations
// must return these (exact) errors (not just an error with the same text).
var (
	ErrDSInvalidEntityType     = datastore.ErrInvalidEntityType
	ErrDSInvalidKey            = datastore.ErrInvalidKey
	ErrDSNoSuchEntity          = datastore.ErrNoSuchEntity
	ErrDSConcurrentTransaction = datastore.ErrConcurrentTransaction
	ErrDSQueryDone             = datastore.Done

	ErrMCCacheMiss   = memcache.ErrCacheMiss
	ErrMCCASConflict = memcache.ErrCASConflict
	ErrMCNoStats     = memcache.ErrNoStats
	ErrMCNotStored   = memcache.ErrNotStored
	ErrMCServerError = memcache.ErrServerError

	ErrTQTaskAlreadyAdded = taskqueue.ErrTaskAlreadyAdded
)

/////////////////////////////// Supporting code ////////////////////////////////

// ErrDSFieldMismatch is returned when a field is to be loaded into a different
// type than the one it was stored from, or when a field is missing or
// unexported in the destination struct.
// StructType is the type of the struct pointed to by the destination argument
// passed to Get or to Iterator.Next.
type ErrDSFieldMismatch struct {
	StructType reflect.Type
	FieldName  string
	Reason     string
}

func (e *ErrDSFieldMismatch) Error() string {
	return fmt.Sprintf("gae: cannot load field %q into a %q: %s",
		e.FieldName, e.StructType, e.Reason)
}

// MultiError is returned by batch operations when there are errors with
// particular elements. Errors will be in a one-to-one correspondence with
// the input elements; successful elements will have a nil entry.
type MultiError []error

func (m MultiError) Error() string {
	s, n := "", 0
	for _, e := range m {
		if e != nil {
			if n == 0 {
				s = e.Error()
			}
			n++
		}
	}
	switch n {
	case 0:
		return "(0 errors)"
	case 1:
		return s
	case 2:
		return s + " (and 1 other error)"
	}
	return fmt.Sprintf("%s (and %d other errors)", s, n-1)
}

// SingleError provides a simple way to uwrap a MultiError if you know that it
// could only ever contain one element.
//
// If err is a MultiError, return its first element. Otherwise, return err.
func SingleError(err error) error {
	if me, ok := err.(MultiError); ok {
		if len(me) == 0 {
			return nil
		}
		return me[0]
	}
	return err
}

var (
	multiErrorType = reflect.TypeOf(MultiError(nil))
)

// FixError will convert a backend-specific non-plain error type to the
// corresponding gae wrapper type. This is intended to be used solely by
// implementations (not user code). A correct implementation of the gae wrapper
// should never return an SDK-specific error type if an alternate type appears
// in this file.
func FixError(err error) error {
	if err != nil {
		// we know that err already conforms to the error interface (or the caller's
		// method wouldn't compile), so check to see if the error's underlying type
		// looks like one of the special error types we implement.
		v := reflect.ValueOf(err)
		if v.Type().ConvertibleTo(multiErrorType) {
			err = v.Convert(multiErrorType).Interface().(error)
		}
	}
	return err
}

// LazyMultiError is a lazily-constructed MultiError. You specify the target
// MultiError size up front (as Size), and then you call Assign for each error
// encountered, and it's potential index. The MultiError will only be allocated
// if one of the Assign'd errors is non-nil. Similarly, Get will retrieve either
// the allocated MultiError, or nil if no error was encountered.
type LazyMultiError struct {
	sync.Mutex

	Size int
	me   MultiError
}

// Assign semantically assigns the error to the given index in the MultiError.
// If the error is nil, no action is taken. Otherwise the MultiError is
// allocated to its full size (if not already), and the error assigned into it.
func (e *LazyMultiError) Assign(i int, err error) {
	if err == nil {
		return
	}
	e.Lock()
	defer e.Unlock()
	if e.me == nil {
		e.me = make(MultiError, e.Size)
	}
	e.me[i] = err
}

// Get returns the MultiError, or nil, if no non-nil error was Assign'd.
func (e *LazyMultiError) Get() error {
	e.Lock()
	defer e.Unlock()
	if e.me == nil {
		return nil
	}
	return e.me
}

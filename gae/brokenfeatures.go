// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// FeatureBreaker allows a fake implementation to set and unset broken features.
// A feature is the Name of some method on the fake. So if you had:
//   var fake interface{ FeatureBreaker, MCSingleReadWriter } = ...
//
// you could do:
//   fake.BreakFeatures(memcache.ErrServerError, "Add", "Set")
//
// and then
//   fake.Add(...) and fake.Set(...)
//
// would return the error.
//
// You may also pass nil as the error for BreakFeatures, and the fake will
// provide some suitable (but generic) error for those features (like a
// BAD_REQUEST or something like that).
type FeatureBreaker interface {
	BreakFeatures(err error, feature ...string)
	UnbreakFeatures(feature ...string)
}

// ErrBrokenFeaturesBroken is returned from IsBroken when BrokenFeatures itself
// isn't working correctly.
var ErrBrokenFeaturesBroken = errors.New("brokenFeatures: Unable to retrieve caller information")

// BrokenFeatures implements the FeatureBreaker interface, and is suitable for
// embedding within a fake service.
type BrokenFeatures struct {
	lock sync.Mutex

	broken map[string]error

	// DefaultError is the default error to return when you call
	// BreakFeatures(nil, ...). If this is unset and the user calls BreakFeatures
	// with nil, BrokenFeatures will return a generic error.
	DefaultError error
}

// BreakFeatures allows you to specify an MCSingleReadWriter function name
// to cause it to return memcache.ErrServerError. e.g.
//
//   m.SetBrokenFeatures("Add")
//
// would return memcache.ErrServerError. You can reverse this by calling
// UnbreakFeatures("Add").
func (b *BrokenFeatures) BreakFeatures(err error, feature ...string) {
	b.lock.Lock()
	defer b.lock.Unlock()
	if b.broken == nil {
		b.broken = map[string]error{}
	}

	for _, f := range feature {
		b.broken[f] = err
	}
}

// UnbreakFeatures is the inverse of BreakFeatures, and will return the named
// features back to their original functionality.
func (b *BrokenFeatures) UnbreakFeatures(feature ...string) {
	b.lock.Lock()
	defer b.lock.Unlock()

	for _, f := range feature {
		delete(b.broken, f)
	}
}

// IsBroken is to be called internally by the fake service on every
// publically-facing method. If it returns an error, the fake should return
// the error.
//
// Example:
//   type MyService struct { BrokenFeatures }
//   func (ms *MyService) Thingy() error {
//     if err := ms.IsBroken(); err != nil {
//       return err
//     }
//		 ...
//   }
//
//  You can now do ms.SetBrokenFeatures("Thingy"), and Thingy will return an
//  error.
//
//  Note that IsBroken will keep walking the stack until it finds the first
//  publically-exported method, which will allow you to put the IsBroken call
//  in an internal helper method of your service implementation.
//
//  Additionaly, IsBroken allows a very primitive form of overriding; it walks
//  the stack until it finds the first method which is not called "IsBroken".
//  This allows the embedding struct to call into BrokenFeatures.IsBroken from
//  another IsBroken function, and still have it behave correctly.
func (b *BrokenFeatures) IsBroken() error {
	if b.noBrokenFeatures() {
		return nil
	}

	var name string
	for off := 1; ; off++ { // offset of 1 skips ourselves by default
		// TODO(riannucci): Profile this to see if it's having an adverse
		// performance impact ont tests.
		fn, _, _, ok := runtime.Caller(off)
		if !ok {
			return ErrBrokenFeaturesBroken
		}
		toks := strings.Split(runtime.FuncForPC(fn).Name(), ".")
		name = toks[len(toks)-1]
		firstRune, _ := utf8.DecodeRuneInString(name)
		if !unicode.IsUpper(firstRune) {
			// unexported method, keep walking till we find the first exported
			// method. Do !IsUpper, since exported is defined by IsUpper and not
			// !IsLower, and afaik, in unicode-land they're not direct opposites.
			continue
		}
		if name == "IsBroken" {
			// Allow users to override IsBroken, keep walking until we see a function
			// which is named differently than IsBroken.
			continue
		}
		break
	}

	b.lock.Lock()
	defer b.lock.Unlock()
	if err, ok := b.broken[name]; ok {
		if err != nil {
			return err
		}
		if b.DefaultError != nil {
			return b.DefaultError
		}
		return fmt.Errorf("feature %q is broken", name)
	}

	return nil
}

func (b *BrokenFeatures) noBrokenFeatures() bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	return len(b.broken) == 0
}

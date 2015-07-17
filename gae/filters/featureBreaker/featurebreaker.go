// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package featureBreaker

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
)

// FeatureBreaker is the state-access interface for all Filter* functions in
// this package.  A feature is the Name of some method on the filtered service.
// So if you had:
//   c, fb := FilterMC(...)
//   mc := gae.GetMC(c)
//
// you could do:
//   fb.BreakFeatures(memcache.ErrServerError, "Add", "Set")
//
// and then
//   mc.Add(...) and mc.Set(...)
//
// would return the error.
//
// You may also pass nil as the error for BreakFeatures, and the fake will
// provide the DefaultError which you passed to the Filter function.
//
// This interface can only break features which return errors.
type FeatureBreaker interface {
	BreakFeatures(err error, feature ...string)
	UnbreakFeatures(feature ...string)
}

// ErrBrokenFeaturesBroken is returned from RunIfNotBroken when BrokenFeatures
// itself isn't working correctly.
var ErrBrokenFeaturesBroken = errors.New("featureBreaker: Unable to retrieve caller information")

type state struct {
	sync.Mutex

	broken map[string]error

	// defaultError is the default error to return when you call
	// BreakFeatures(nil, ...). If this is unset and the user calls BreakFeatures
	// with nil, BrokenFeatures will return a generic error.
	defaultError error
}

func newState(dflt error) *state {
	return &state{
		broken:       map[string]error{},
		defaultError: dflt,
	}
}

// BreakFeatures allows you to specify an MCSingleReadWriter function name
// to cause it to return memcache.ErrServerError. e.g.
//
//   m.SetBrokenFeatures("Add")
//
// would return memcache.ErrServerError. You can reverse this by calling
// UnbreakFeatures("Add").
func (s *state) BreakFeatures(err error, feature ...string) {
	s.Lock()
	defer s.Unlock()
	for _, f := range feature {
		s.broken[f] = err
	}
}

// UnbreakFeatures is the inverse of BreakFeatures, and will return the named
// features back to their original functionality.
func (s *state) UnbreakFeatures(feature ...string) {
	s.Lock()
	defer s.Unlock()
	for _, f := range feature {
		delete(s.broken, f)
	}
}

func (s *state) run(f func() error) error {
	if s.noBrokenFeatures() {
		return f()
	}

	pc, _, _, _ := runtime.Caller(1)
	fullName := runtime.FuncForPC(pc).Name()
	fullNameParts := strings.Split(fullName, ".")
	name := fullNameParts[len(fullNameParts)-1]

	s.Lock()
	err, ok := s.broken[name]
	dflt := s.defaultError
	s.Unlock()

	if ok {
		if err != nil {
			return err
		}
		if dflt != nil {
			return dflt
		}
		return fmt.Errorf("feature %q is broken", name)
	}

	return f()
}

func (s *state) noBrokenFeatures() bool {
	s.Lock()
	defer s.Unlock()
	return len(s.broken) == 0
}

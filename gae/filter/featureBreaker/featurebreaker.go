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

package featureBreaker

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/net/context"
)

// BreakFeatureCallback can be used to break features at the time of the call.
//
// If it return an error, this error will be returned by the corresponding API
// call as is. If returns nil, the call will be performed as usual.
//
// It receives a derivative of a context passed to the original API call which
// can be used to extract any contextual information, if necessary.
//
// The callback will be called often and concurrently. Provide your own
// synchronization if necessary.
type BreakFeatureCallback func(ctx context.Context, feature string) error

// FeatureBreaker is the state-access interface for all Filter* functions in
// this package.  A feature is the Name of some method on the filtered service.
//
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
	// BreakFeatures allows you to set an error that should be returned by the
	// corresponding functions.
	//
	// For example
	//   m.BreakFeatures(memcache.ErrServerError, "Add")
	//
	// would make memcache.Add return memcache.ErrServerError. You can reverse
	// this by calling UnbreakFeatures("Add").
	//
	// The only exception to this rule is two "fake" functions that can be used
	// to simulate breaking "RunInTransaction" in a more detailed way:
	//   * Use "BeginTransaction" as a feature name to simulate breaking of a new
	//     transaction attempt. It is called before each individual retry.
	//   * Use "CommitTransaction" as a feature name to simulate breaking the
	//     transaction commit RPC. It is called after the transaction body
	//     completes. Returning datastore.ErrConcurrentTransaction here will cause
	//     a retry.
	//
	// "RunInTransaction" itself is not breakable. Break "BeginTransaction" or
	// "CommitTransaction" instead.
	BreakFeatures(err error, feature ...string)

	// BreakFeaturesWithCallback is like BreakFeatures, except it allows you to
	// decide whether to return an error or not at the time the function call is
	// happening.
	//
	// The callback will be called often and concurrently. Provide your own
	// synchronization if necessary.
	//
	// You may use a callback returned by flaky.Errors(...) to emulate randomly
	// occurring errors.
	//
	// Note that the default error passed to Filter* functions are ignored when
	// using callbacks.
	BreakFeaturesWithCallback(cb BreakFeatureCallback, feature ...string)

	// UnbreakFeatures is the inverse of BreakFeatures/BreakFeaturesWithCallback,
	// and will return the named features back to their original functionality.
	UnbreakFeatures(feature ...string)
}

// errUseDefault is never returned but used as an indicator to use defaultError.
var errUseDefault = errors.New("use default error")

type state struct {
	l      sync.RWMutex
	broken map[string]BreakFeatureCallback

	// defaultError is the default error to return when you call
	// BreakFeatures(nil, ...). If this is unset and the user calls BreakFeatures
	// with nil, BrokenFeatures will return a generic error.
	defaultError error
}

func newState(dflt error) *state {
	return &state{
		broken:       map[string]BreakFeatureCallback{},
		defaultError: dflt,
	}
}

func (s *state) BreakFeatures(err error, feature ...string) {
	if err == nil {
		err = errUseDefault
	}
	s.BreakFeaturesWithCallback(
		func(context.Context, string) error { return err },
		feature...)
}

func (s *state) BreakFeaturesWithCallback(cb BreakFeatureCallback, feature ...string) {
	for _, f := range feature {
		if f == "RunInTransaction" {
			panic("break BeginTransaction or CommitTransaction instead of RunInTransaction")
		}
	}
	s.l.Lock()
	defer s.l.Unlock()
	for _, f := range feature {
		s.broken[f] = cb
	}
}

func (s *state) UnbreakFeatures(feature ...string) {
	s.l.Lock()
	defer s.l.Unlock()
	for _, f := range feature {
		delete(s.broken, f)
	}
}

func (s *state) run(c context.Context, f func() error) error {
	if s.noBrokenFeatures() {
		return f()
	}

	pc, _, _, _ := runtime.Caller(1)
	fullName := runtime.FuncForPC(pc).Name()
	fullNameParts := strings.Split(fullName, ".")
	name := fullNameParts[len(fullNameParts)-1]

	s.l.RLock()
	cb := s.broken[name]
	dflt := s.defaultError
	s.l.RUnlock()

	if cb == nil {
		return f()
	}

	switch err := cb(c, name); {
	case err == nil: // the callback decided not to break the feature
		return f()
	case err == errUseDefault:
		if dflt != nil {
			return dflt
		}
		return fmt.Errorf("feature %q is broken", name)
	default:
		return err
	}
}

func (s *state) noBrokenFeatures() bool {
	s.l.RLock()
	defer s.l.RUnlock()
	return len(s.broken) == 0
}

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

import (
	"time"

	"golang.org/x/net/context"
)

// TimeNowFactory is the function signature for factory methods compatible with
// SetTimeNowFactory.
type TimeNowFactory func(context.Context) time.Time

// GetTimeNow gets the current time.Time from the context. If the context has no
// time factory set with SetTimeNowFactory, it returns time.Now().
func GetTimeNow(c context.Context) time.Time {
	if f, ok := c.Value(timeNowKey).(TimeNowFactory); ok && f != nil {
		return f(c)
	}
	return time.Now()
}

// SetTimeNowFactory sets the function to produce time.Time instances, as
// returned by the GetTimeNow method.
func SetTimeNowFactory(c context.Context, tnf TimeNowFactory) context.Context {
	return context.WithValue(c, timeNowKey, tnf)
}

// SetTimeNow sets a pointer to the current time.Time object in the context.
// Useful for testing with a quick mock. This is just a shorthand
// SetTimeNowFactory invocation to set a factory which always returns the same
// object.
//
// Note the pointer asymmetry with GetTimeNow. This is done intentionally to
// allow you to modify the pointed-to-time to be able to manually monkey with
// the clock in a test. Otherwise this would essentially freeze time in place.
// If you pass 'nil', this will set the context back to returning time.Now().
func SetTimeNow(c context.Context, t *time.Time) context.Context {
	if t == nil {
		return SetTimeNowFactory(c, nil)
	}
	return SetTimeNowFactory(c, func(context.Context) time.Time { return *t })
}

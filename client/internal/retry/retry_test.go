// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package retry

import (
	"errors"
	"testing"
	"time"

	"github.com/maruel/ut"
)

func TestDoOnce(t *testing.T) {
	c := &Config{1, 0, 0, 0}
	r := &retriable{}
	ut.AssertEqual(t, nil, c.Do(r))
	ut.AssertEqual(t, 1, r.closed)
	ut.AssertEqual(t, 1, r.tries)
}

func TestDoRetryExceeded(t *testing.T) {
	c := &Config{1, 0, 0, 0}
	r := &retriable{errs: []error{errRetry}}
	ut.AssertEqual(t, errRetry, c.Do(r))
	ut.AssertEqual(t, 1, r.closed)
	ut.AssertEqual(t, 1, r.tries)
}

func TestDoRetry(t *testing.T) {
	c := &Config{2, 0, time.Millisecond, 0}
	r := &retriable{errs: []error{errRetry}}
	ut.AssertEqual(t, nil, c.Do(r))
	ut.AssertEqual(t, 1, r.closed)
	ut.AssertEqual(t, 2, r.tries)
}

func TestError(t *testing.T) {
	ut.AssertEqual(t, "please try again", errRetry.Error())
}

// Private details.

var errYo = errors.New("yo")
var errRetry = Error{errors.New("please try again")}

type retriable struct {
	closed int
	tries  int
	errs   []error
}

func (r *retriable) Close() error {
	r.closed++
	if r.closed > 1 {
		return errYo
	}
	return nil
}

func (r *retriable) Do() error {
	r.tries++
	if len(r.errs) != 0 {
		err := r.errs[0]
		r.errs = r.errs[1:]
		return err
	}
	return nil
}

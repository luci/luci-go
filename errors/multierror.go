// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package errors

import (
	"fmt"
)

// MultiError is a simple `error` implementation which represents multiple
// `error` objects in one.
type MultiError []error

// MultiErrorFromErrors takes an error-channel, blocks on it, and returns
// a MultiError for any errors pushed to it over the channel, or nil if
// all the errors were nil.
func MultiErrorFromErrors(ch <-chan error) error {
	if ch == nil {
		return nil
	}
	ret := MultiError(nil)
	for e := range ch {
		if e == nil {
			continue
		}
		ret = append(ret, e)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func (m MultiError) Error() string {
	return fmt.Sprintf("%+q", []error(m))
}

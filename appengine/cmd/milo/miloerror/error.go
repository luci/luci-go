// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package miloerror

import (
	"fmt"
)

// Error is used for all milo errors.
type Error struct {
	Message string
	Code    int
}

func (e *Error) Error() string {
	return fmt.Sprintf("encountered error %d, %s", e.Code, e.Message)
}

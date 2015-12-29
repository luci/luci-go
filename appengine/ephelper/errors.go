// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ephelper

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
)

// StripError converts the supplied error into an endpoints.APIError.
//
// If the error is not already an endpoints.APIError, InternalServerError
// will be returned.
func StripError(err error) *endpoints.APIError {
	if err == nil {
		return nil
	}

	epErr, ok := err.(*endpoints.APIError)
	if !ok {
		epErr = endpoints.InternalServerError.(*endpoints.APIError)
	}
	return epErr
}

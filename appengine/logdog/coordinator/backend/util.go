// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"fmt"
	"net/http"

	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// httpError
type httpError struct {
	reason error
	code   int
}

func (e *httpError) Error() string {
	if r := e.reason; r != nil {
		return fmt.Sprintf("%v: %q", e.reason, http.StatusText(e.code))
	}
	return http.StatusText(e.code)
}

func newHTTPError(reason error, code int) error {
	return &httpError{reason, code}
}

// errorWrapper wraps an error-returning function and responds with an
// InternalServerError if an error is returned.
func errorWrapper(c context.Context, w http.ResponseWriter, f func() error) {
	if err := f(); err != nil {
		statusCode := http.StatusInternalServerError
		if e, ok := err.(*httpError); ok {
			statusCode = e.code
		}

		log.Fields{
			log.ErrorKey: err,
			"statusCode": statusCode,
		}.Errorf(c, "Backend handler returned error.")
		w.WriteHeader(statusCode)
	}
}

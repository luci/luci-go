// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"errors"
	"net/http"

	"google.golang.org/api/googleapi"
)

var (
	// ErrNoAccess is returned from methods to indicate that the requested
	// operation could not be performed because the user does not have access
	// to that data or function.
	ErrNoAccess = errors.New("coordinator: no access")

	// ErrNoSuchStream is returned when the requested strem path does not exist.
	ErrNoSuchStream = errors.New("coordinator: no such stream")
)

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*googleapi.Error); ok {
		switch e.Code {
		case http.StatusNotFound:
			return ErrNoSuchStream

		case http.StatusUnauthorized:
			fallthrough
		case http.StatusForbidden:
			return ErrNoAccess

		default:
			return e
		}
	}
	return err
}

// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package errors

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
)

// New is a pass-through version of the standard errors.New function.
var New = errors.New

// markedError is the specific error type retuned by MakeMarkFn.
type markedError struct {
	// inner contains the original error which was passed to the marker function.
	inner error

	formatOnce sync.Once
	formatFn   func() string
	msg        string // The formatted error message (protected by formatOnce).
}

var _ Wrapped = (*markedError)(nil)

func (merr *markedError) Error() string {
	merr.formatOnce.Do(func() {
		merr.msg = merr.formatFn()
		merr.formatFn = nil
	})
	return merr.msg
}

func (merr *markedError) InnerError() error {
	return merr.inner
}

// MakeMarkFn returns a new 'marker' function for your library. The name parameter
// should probably match your package name, but it can by any string which
// will help identify the error as originating from your package.
func MakeMarkFn(name string) func(error) error {
	return func(err error) error {
		if err == nil {
			return nil
		}

		_, filename, line, gotInfo := runtime.Caller(1)
		return &markedError{
			inner: err,
			formatFn: func() string {
				if gotInfo {
					return fmt.Sprintf("%s: %s:%d: %+v", name, filepath.Base(filename), line, err.Error())
				}
				return fmt.Sprintf("%s: %+v", name, err.Error())
			},
		}
	}
}

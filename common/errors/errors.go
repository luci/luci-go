// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package errors

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// MarkedError is the specific error type retuned by MakeMarkFn. It's designed
// so that you can access the underlying object if needed.
type MarkedError struct {
	// Orig contains the original data (error or otherwise) which was passed to
	// the marker function.
	Orig interface{}

	msgFmt string
}

func (te *MarkedError) Error() string {
	return fmt.Sprintf(te.msgFmt, te.Orig)
}

// MakeMarkFn returns a new 'marker' function for your library. The name parameter
// should probably match your package name, but it can by any string which
// will help identify the error as originating from your package.
func MakeMarkFn(name string) func(interface{}) error {
	fmtstring1 := name + " - %s:%d - %%+v"
	fmtstring2 := name + " - %+v"
	return func(errish interface{}) error {
		if errish == nil {
			return nil
		}
		_, filename, line, gotInfo := runtime.Caller(1)
		pfx := fmtstring2
		if gotInfo {
			pfx = fmt.Sprintf(fmtstring1, filepath.Base(filename), line)
		}
		return &MarkedError{errish, pfx}
	}
}

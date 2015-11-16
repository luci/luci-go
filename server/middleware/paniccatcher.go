// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package middleware

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/paniccatcher"
)

// WithPanicCatcher is a middleware that catches panics, dumps stack trace to
// logging and returns HTTP 500.
func WithPanicCatcher(h Handler) Handler {
	return func(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		catcher := &paniccatcher.Wrapper{}
		defer func() {
			if catcher.DidPanic() {
				// Note: it may be too late to send HTTP 500 if `h` already sent
				// headers. But there's nothing else we can do at this point anyway.
				http.Error(w, "Internal Server Error. See logs.", http.StatusInternalServerError)
			}
		}()
		defer catcher.Catch(c, "Caught panic during handling of %q", r.RequestURI)
		h(c, w, r, p)
	}
}

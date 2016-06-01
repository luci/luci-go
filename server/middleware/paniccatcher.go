// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/paniccatcher"
)

// WithPanicCatcher is a middleware that catches panics, dumps stack trace to
// logging and returns HTTP 500.
func WithPanicCatcher(h Handler) Handler {
	return func(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		defer paniccatcher.Catch(func(p *paniccatcher.Panic) {
			log.Fields{
				"panic.error": p.Reason,
			}.Errorf(c, "Caught panic during handling of %q:\n%s", r.RequestURI, p.Stack)

			// Note: it may be too late to send HTTP 500 if `h` already sent
			// headers. But there's nothing else we can do at this point anyway.
			http.Error(w, "Internal Server Error. See logs.", http.StatusInternalServerError)
		})
		h(c, w, r, p)
	}
}

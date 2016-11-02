// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package prod

import (
	"net/http"

	uf "github.com/luci/gae/service/urlfetch"
	"golang.org/x/net/context"
	"google.golang.org/appengine/urlfetch"
)

// useURLFetch adds a http.RoundTripper implementation to the context.
func useURLFetch(c context.Context) context.Context {
	return uf.SetFactory(c, func(ci context.Context) http.RoundTripper {
		return &urlfetch.Transport{Context: getAEContext(ci)}
	})
}

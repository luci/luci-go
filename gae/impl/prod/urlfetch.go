// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
		return &urlfetch.Transport{Context: ci}
	})
}

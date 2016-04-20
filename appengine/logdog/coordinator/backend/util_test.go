// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/server/middleware"
	"golang.org/x/net/context"
)

// testBase is a middleware.Base which uses its current Context as the base
// context.
type testBase struct {
	context.Context
}

func (t *testBase) base(h middleware.Handler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		h(t.Context, w, r, p)
	}
}

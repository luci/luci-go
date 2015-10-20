// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package middleware defines base type for context-aware HTTP request handler.
// See appengine/middleware for examples of how to use it in GAE environment.
package middleware

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
)

// Handler is the type for all request handlers. Of particular note, it's the
// same as httprouter.Handle, except that it also has a context parameter.
type Handler func(context.Context, http.ResponseWriter, *http.Request, httprouter.Params)

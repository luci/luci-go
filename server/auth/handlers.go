// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/middleware"
)

// InstallHandlers installs HTTP handlers that return information useful for
// debugging authentication issues.
//
// This is optional. If you using appengine/gaeauth/server, these handlers are
// already installed.
func InstallHandlers(r *httprouter.Router, base middleware.Base) {
	r.GET("/auth/api/v1/accounts/self", base(Authenticate(accountsSelfHandler)))
}

// accountsSelfHandler returns JSON with information about the caller.
//
// It can be used to verify callers' IP, access tokens, etc.
func accountsSelfHandler(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
	var reply struct {
		Error    string `json:"error,omitempty"`
		Identity string `json:"identity,omitempty"`
		IP       string `json:"ip,omitempty"`
	}

	state := GetState(c)
	if state == nil {
		reply.Error = "Auth state is not available, application is probably using auth library wrong."
	} else {
		reply.Identity = string(state.User().Identity)
		reply.IP = state.PeerIP().String()
	}

	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	if reply.Error == "" {
		rw.WriteHeader(http.StatusOK)
	} else {
		rw.WriteHeader(http.StatusNotImplemented)
		logging.Errorf(c, "HTTP 501 - %s", reply.Error)
	}
	json.NewEncoder(rw).Encode(&reply)
}

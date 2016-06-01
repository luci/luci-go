// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package info exposes /auth/api/v1/server/info URL and allows to query it.
//
// This endpoint is used by luci services to advertise service account names
// they use (among other things).
package info

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/internal"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/proccache"
)

// ServiceInfo describes JSON format of /auth/api/v1/server/info response.
type ServiceInfo struct {
	AppID              string `json:"app_id,omitempty"`
	AppRuntime         string `json:"app_runtime,omitempty"`
	AppRuntimeVersion  string `json:"app_runtime_version,omitempty"`
	AppVersion         string `json:"app_version,omitempty"`
	ServiceAccountName string `json:"service_account_name,omitempty"`
}

type proccacheKey string

// FetchServiceInfo fetches information about the service at given URL.
//
// Uses proccache to cache it locally for 1 hour.
func FetchServiceInfo(c context.Context, serviceURL string) (*ServiceInfo, error) {
	info, err := proccache.GetOrMake(c, proccacheKey(serviceURL), func() (interface{}, time.Duration, error) {
		info := &ServiceInfo{}
		err := internal.FetchJSON(c, info, func() (*http.Request, error) {
			return http.NewRequest("GET", serviceURL+"/auth/api/v1/server/info", nil)
		})
		if err != nil {
			return nil, 0, err
		}
		return info, time.Hour, nil
	})
	if err != nil {
		return nil, err
	}
	return info.(*ServiceInfo), nil
}

// ServiceInfoCallback is called by /auth/api/v1/server/info handler.
type ServiceInfoCallback func(context.Context) (ServiceInfo, error)

// InstallHandlers installs handler that serves info provided by the callback.
func InstallHandlers(r *httprouter.Router, base middleware.Base, cb ServiceInfoCallback) {
	handler := func(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		var response struct {
			ServiceInfo
			Error string `json:"error,omitempty"`
		}
		info, err := cb(c)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			response.Error = err.Error()
		} else {
			w.WriteHeader(http.StatusOK)
			response.ServiceInfo = info
		}
		json.NewEncoder(w).Encode(response)
	}
	r.GET("/auth/api/v1/server/info", base(handler))
}

// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package signing

import (
	"net/http"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/internal"
	"github.com/luci/luci-go/server/proccache"
)

// ServiceInfo describes identity of some service.
//
// It matches JSON format of /auth/api/v1/server/info endpoint.
type ServiceInfo struct {
	AppID              string `json:"app_id,omitempty"`
	AppRuntime         string `json:"app_runtime,omitempty"`
	AppRuntimeVersion  string `json:"app_runtime_version,omitempty"`
	AppVersion         string `json:"app_version,omitempty"`
	ServiceAccountName string `json:"service_account_name,omitempty"`
}

type serviceInfoKey string

// FetchServiceInfo fetches information about the service at given URL.
//
// Uses proccache to cache it locally for 1 hour.
func FetchServiceInfo(c context.Context, serviceURL string) (*ServiceInfo, error) {
	info, err := proccache.GetOrMake(c, serviceInfoKey(serviceURL), func() (interface{}, time.Duration, error) {
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

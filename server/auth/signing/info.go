// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package signing

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth/internal"
	"go.chromium.org/luci/server/caching"
)

// URL string => *ServiceInfo.
var infoCache = caching.RegisterLRUCache(256)

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

// FetchServiceInfo fetches information about the service from the given URL.
//
// The server is expected to reply with JSON described by ServiceInfo struct
// (like LUCI services do). Uses process cache to cache the response for 1h.
//
// LUCI services serve the service info at /auth/api/v1/server/info.
func FetchServiceInfo(c context.Context, url string) (*ServiceInfo, error) {
	info, err := infoCache.LRU(c).GetOrCreate(c, url, func() (interface{}, time.Duration, error) {
		info := &ServiceInfo{}
		req := internal.Request{
			Method: "GET",
			URL:    url,
			Out:    info,
		}
		if err := req.Do(c); err != nil {
			return nil, 0, err
		}
		return info, time.Hour, nil
	})
	if err != nil {
		return nil, err
	}
	return info.(*ServiceInfo), nil
}

// FetchServiceInfoFromLUCIService is shortcut for FetchServiceInfo that uses
// LUCI-specific endpoint.
//
// 'serviceURL' is root URL of the service (e.g. 'https://example.com').
func FetchServiceInfoFromLUCIService(c context.Context, serviceURL string) (*ServiceInfo, error) {
	serviceURL = strings.ToLower(serviceURL)
	if !strings.HasPrefix(serviceURL, "https://") {
		return nil, fmt.Errorf("not an https:// URL - %q", serviceURL)
	}
	domain := strings.TrimPrefix(serviceURL, "https://")
	if domain == "" || strings.ContainsRune(domain, '/') {
		return nil, fmt.Errorf("not a root URL - %q", serviceURL)
	}
	return FetchServiceInfo(c, serviceURL+"/auth/api/v1/server/info")
}

// FetchLUCIServiceIdentity returns "service:<app-id>" of a LUCI service.
//
// It is the same thing as inf.AppID returned by FetchServiceInfoFromLUCIService
// except it is cached more aggressively because service ID is static (unlike
// some other ServiceInfo fields).
//
// 'serviceURL' is root URL of the service (e.g. 'https://example.com').
func FetchLUCIServiceIdentity(c context.Context, serviceURL string) (identity.Identity, error) {
	info, err := FetchServiceInfoFromLUCIService(c, serviceURL)
	if err != nil {
		return "", err
	}
	return identity.MakeIdentity("service:" + info.AppID)
}

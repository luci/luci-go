// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/ephelper"
)

var (
	// Scopes is the set of OAuth scopes required by the service endpoints.
	Scopes = []string{
		endpoints.EmailScope,
	}

	// Info is the service info for this endpoint.
	Info = endpoints.ServiceInfo{
		Version:     "v1",
		Description: "LogDog Admin API",
	}

	// MethodInfoMap is the method info map for the Admin endpoint service.
	MethodInfoMap = ephelper.MethodInfoMap{
		"SetConfig": &endpoints.MethodInfo{
			Desc:   "Set the instance's global configuration parameters.",
			Name:   "setConfig",
			Path:   "setConfig",
			Scopes: Scopes,
		},
	}
)

// Admin is the Cloud Endpoint service structure for the administrator endpoint.
type Admin struct {
	ephelper.ServiceBase
}

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/ephelper"
)

// DungeonMaster is the endpoints server object.
type DungeonMaster struct {
	ephelper.ServiceBase
}

// MethodInfo provides the method info map that each service
// registers itself in.
var MethodInfo = ephelper.MethodInfoMap{}

// DungeonMasterServiceInfo is the service-wide endpoints configuration.
var DungeonMasterServiceInfo = &endpoints.ServiceInfo{
	Name:        "dm",
	Version:     "v1",
	Description: "DungeonMaster task scheduling service",
	Default:     true,
}

// RegisterEndpointsService is used to integrate with ephelper.
func RegisterEndpointsService(srv *endpoints.Server) error {
	return ephelper.Register(srv, &DungeonMaster{},
		DungeonMasterServiceInfo, MethodInfo)
}

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dumbCounter

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/ephelper"
)

var (
	// MethodInfoMap is the collection of MethodInfo for the service's endpoints.
	MethodInfoMap = ephelper.MethodInfoMap{
		"List":         listMethodInfo,
		"Add":          addMethodInfo,
		"CAS":          casMethodInfo,
		"CurrentValue": currentValueMethodInfo,
	}

	// ServiceInfo is information about the Example service.
	ServiceInfo = &endpoints.ServiceInfo{
		Name:        "dumb_counter",
		Version:     "v1",
		Description: "A hideously stupid persistant counter service.",
	}
)

// Example is the example service type.
type Example struct {
	ephelper.ServiceBase
}

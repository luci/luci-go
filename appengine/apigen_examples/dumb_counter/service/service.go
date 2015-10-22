// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dumbCounter is an example Google Cloud Endpoints service.
//
// It implements a really dumb named persistant counter.
package dumbCounter

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/ephelper"
)

// Example is the example service type.
type Example struct{}

var mi = ephelper.MethodInfoMap{
	"List":         listMethodInfo,
	"Add":          addMethodInfo,
	"CAS":          casMethodInfo,
	"CurrentValue": currentValueMethodInfo,
}

var si = &endpoints.ServiceInfo{
	Name:        "dumb_counter",
	Version:     "v1",
	Description: "A hideously stupid persistant counter service.",
}

// Uncomment this block to make this a Managed VM application.
func init() {
	ephelper.Register(endpoints.DefaultServer, Example{}, si, mi)
	endpoints.HandleHTTP()
}

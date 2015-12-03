// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package service is an example Google Cloud Endpoints service.
//
// It implements a really dumb named persistant counter.
package service

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/apigen_examples/dumb_counter/dumbCounter"
	"github.com/luci/luci-go/appengine/ephelper"
)

// Uncomment this block to make this a Managed VM application.
func init() {
	ephelper.Register(endpoints.DefaultServer, &dumbCounter.Example{}, dumbCounter.ServiceInfo, dumbCounter.MethodInfoMap)
	endpoints.HandleHTTP()
}

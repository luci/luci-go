// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package apiservers

import (
	"github.com/luci/luci-go/scheduler/api/scheduler/v1"
	"github.com/luci/luci-go/scheduler/appengine/catalog"
	"github.com/luci/luci-go/scheduler/appengine/engine"
)

// SchedulerServer implements scheduler.Scheduler API.
type SchedulerServer struct {
	Engine  engine.Engine
	Catalog catalog.Catalog
}

var _ scheduler.SchedulerServer = (*SchedulerServer)(nil)

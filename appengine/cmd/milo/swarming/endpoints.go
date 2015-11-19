// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package swarming

import (
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"golang.org/x/net/context"
)

// BuildReq is a request for a build on swarming.
type BuildReq struct {
	SwarmingID string
}

// BuildLogReq is a request for a build log from swarming.
type BuildLogReq struct {
	SwarmingID string
	StepName   string
	LogName    string
}

// BuildLog contains the log text retrieved from swarming.
// TODO(hinoka): Maybe put this somewhere more generic, like under resp/.
type BuildLog struct {
	log string
}

// Service is the endpoint API.
type Service struct{}

// Build returns the build for the given BuildReq (the swarming ID).
func (ss *Service) Build(c context.Context, r *BuildReq) (*resp.MiloBuild, error) {
	return swarmingBuildImpl(c, "foo", r.SwarmingID)
}

// BuildLog gets the requested build log from swarming.
func (ss *Service) BuildLog(c context.Context, r *BuildLogReq) (*BuildLog, error) {
	return swarmingBuildLogImpl(c, r.SwarmingID, r.LogName, r.StepName)
}

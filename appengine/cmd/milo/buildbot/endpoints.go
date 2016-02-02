// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

import (
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"golang.org/x/net/context"
)

// BuildReq is a request for a build on buildbot.
type BuildReq struct {
	Master  string
	Builder string
	Build   string
}

// BuildLogReq is a request for a build log from swarming.
type BuildLogReq struct {
	BuildReq
	step string
	log  string
}

// BuildLog contains the log text retrieved from swarming.
// TODO(hinoka): Maybe put this somewhere more generic, like under resp/.
type BuildLog struct {
	log string
}

// Service is the endpoint API.
type Service struct{}

// Build returns the build for the given BuildReq (the canonical buildbot URL).
func (ss *Service) Build(c context.Context, r *BuildReq) (*resp.MiloBuild, error) {
	return build(c, r.Master, r.Builder, r.Build)
}

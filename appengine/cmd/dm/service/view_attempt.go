// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"time"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"golang.org/x/net/context"
)

// ViewAttemptReq is the input parameters for the ViewAttempt API.
type ViewAttemptReq struct {
	types.AttemptID

	Options *AttemptOpts

	TimeoutMs uint32 // 0 == max timeout (50 seconds)
}

// ViewAttempt allows you to walk the Attempt graph, starting at a given
// Attempt, and walking until you hit the end of the graph, or it hits the
// TimeoutMs. At this point, all data collected will be returned
func (d *DungeonMaster) ViewAttempt(c context.Context, req *ViewAttemptReq) (ret *display.Data, err error) {
	if c, err = d.Use(c, MethodInfo["ViewAttempt"]); err != nil {
		return
	}

	// TODO(iannucci): restrict this against current executions (or filter by
	// attempts the current execution is allowed to see). Basically, we don't
	// want an execution to be able to cheat and view results of attempts which
	// are not dependend on by directly that attempt.
	atmpt := &model.Attempt{AttemptID: req.AttemptID}
	err = datastore.Get(c).Get(atmpt)
	if err != nil {
		return
	}

	const max = time.Second * 50
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout > max || timeout == 0 {
		timeout = max
	}
	c, _ = context.WithTimeout(c, timeout)

	ret = WalkAttempts(c, req.AttemptID, req.Options)

	return
}

func init() {
	MethodInfo["ViewAttempt"] = &endpoints.MethodInfo{
		Name:       "quests.attempts.get",
		HTTPMethod: "GET",
		Path:       "quests/{QuestID}/attempts/{AttemptNum}",
		Desc:       "Get the status and dependencies of an Attempt",
	}
}

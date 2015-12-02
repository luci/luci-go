// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"fmt"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/mutate"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/appengine/tumble"
	"golang.org/x/net/context"
)

// EnsureAttemptReq is used to ensure that a singe Attempt exists.
type EnsureAttemptReq struct {
	types.AttemptID
}

// EnsureAttempt allows you to begin an Attempt directly. This is intended as
// a human-facing api to begin DM Attempt graphs.
//
// You must have already ensured that the target Quest exists with EnsureQuest.
func (d *DungeonMaster) EnsureAttempt(c context.Context, req *EnsureAttemptReq) (err error) {
	if c, err = d.Use(c, MethodInfo["EnsureAttempt"]); err != nil {
		return
	}

	ds := datastore.Get(c)

	if err = ds.Get(&model.Quest{ID: req.QuestID}); err != nil {
		return fmt.Errorf("no such quest %q", req.QuestID)
	}

	return tumble.RunMutation(c, &mutate.EnsureAttempt{ID: req.AttemptID})
}

func init() {
	MethodInfo["EnsureAttempt"] = &endpoints.MethodInfo{
		Name:       "quests.attempts.insert",
		HTTPMethod: "PUT",
		Path:       "quests/{QuestID}/attempts/{AttemptNum}",
		Desc:       "Ensures the existence of an attempt",
	}
}

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"time"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/mutate"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/appengine/tumble"
	"golang.org/x/net/context"
)

// FinishAttemptReq allows an executing Attempt to post its result.
type FinishAttemptReq struct {
	types.AttemptID
	ExecutionKey []byte

	Result           []byte
	ResultExpiration time.Time
}

const resultMaxLength = 256 * 1024

// FinishAttempt allows an executing Attempt to post its result.
func (d *DungeonMaster) FinishAttempt(c context.Context, req *FinishAttemptReq) (err error) {
	if c, err = d.Use(c, MethodInfo["FinishAttempt"]); err != nil {
		return
	}

	req.Result, err = model.NormalizeJSONObject(resultMaxLength, req.Result)
	if err != nil {
		return err
	}

	return tumble.RunMutation(c, &mutate.FinishAttempt{
		ID:               req.AttemptID,
		ExecutionKey:     req.ExecutionKey,
		Result:           req.Result,
		ResultExpiration: req.ResultExpiration,
	})
}

func init() {
	MethodInfo["FinishAttempt"] = &endpoints.MethodInfo{
		Name:       "quests.attempts.finish",
		HTTPMethod: "POST",
		Path:       "quests/{QuestID}/attempts/{AttemptNum}/result",
		Desc:       "Sets the result of an attempt",
	}
}

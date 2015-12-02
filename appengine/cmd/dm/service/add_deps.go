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
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/stringset"
	"golang.org/x/net/context"
)

// AddDepsReq is the request type when adding dependencies to a running attempt.
type AddDepsReq struct {
	types.AttemptID

	To types.AttemptIDSlice

	ExecutionKey []byte `endpoints:"req"`
}

// AddDepsRsp is the response you get when adding dependencies to a running
// attempt. If ShouldHalt is true, your execution key has been revoked and you
// should terminate your execution as quickly as possible.
type AddDepsRsp struct {
	ShouldHalt bool
}

// AddDeps allows you to add attempts to a currently executing Attempt.
func (d *DungeonMaster) AddDeps(c context.Context, req *AddDepsReq) (rsp *AddDepsRsp, err error) {
	if c, err = d.Use(c, MethodInfo["AddDeps"]); err != nil {
		return
	}

	_, _, err = model.VerifyExecution(c, &req.AttemptID, req.ExecutionKey)
	if err != nil {
		return
	}

	rsp = &AddDepsRsp{}
	ds := datastore.Get(c)

	fanout := model.AttemptFanout{
		Base:  &req.AttemptID,
		Edges: req.To,
	}

	// 1. Maybe all the dependencies already exist? If they do, then they must
	// all be completed, or we wouldn't be running the job right now.
	if ds.GetMulti(fanout.Fwds(c)) == nil {
		return
	}

	// 2. Maybe all the target attempts are completed?
	targetQuests := stringset.New(len(fanout.Edges))
	atmpts := make([]*model.Attempt, len(fanout.Edges))
	for i, aid := range fanout.Edges {
		atmpts[i] = &model.Attempt{AttemptID: *aid}
		targetQuests.Add(aid.QuestID)
	}
	_ = ds.GetMulti(atmpts)
	allFinished := true
	for _, a := range atmpts {
		if a == nil || a.State != types.Finished {
			allFinished = false
			break
		}
	}
	if allFinished {
		err = tumble.RunMutation(c, &mutate.AddFinishedDeps{
			ToAdd: &fanout, ExecutionKey: req.ExecutionKey})
		return
	}

	// Ok, I guess we need to halt. Let's at least make sure all the target
	// quests exist first.
	quests := make([]*model.Quest, 0, targetQuests.Len())
	targetQuests.Iter(func(qid string) bool {
		quests = append(quests, &model.Quest{ID: qid})
		return true
	})
	err = ds.GetMulti(quests)
	if err != nil {
		rsp = nil
		if merr, ok := err.(errors.MultiError); ok {
			badQuests := errors.MultiError{}
			for i, e := range merr {
				if e != nil {
					badQuests = append(badQuests, fmt.Errorf("could not load quest %q", quests[i].ID))
				}
			}
			err = badQuests
		}
	} else {
		rsp.ShouldHalt = true
		err = tumble.RunMutation(c, &mutate.AddDeps{
			ToAdd:        &fanout,
			ExecutionKey: req.ExecutionKey,
		})
	}
	return
}

func init() {
	MethodInfo["AddDeps"] = &endpoints.MethodInfo{
		Name:       "quests.attempts.dependencies",
		HTTPMethod: "PUT",
		Path:       "quests/{QuestID}/attempts/{AttemptNum}/dependencies",
		Desc:       "Allows an Execution to add additional dependencies",
	}
}

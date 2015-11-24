// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"fmt"
	"sort"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/gae/impl/prod"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/gaelogger"
	"golang.org/x/net/context"
)

// ViewAttemptsReq is the request for viewing the Attempts which belong to a
// Quest.
type ViewAttemptsReq struct {
	QuestID string
}

// ViewAttempts allows you to view the Attempts that exist for a Quest.
func (d *DungeonMaster) ViewAttempts(c context.Context, req *ViewAttemptsReq) (*display.Data, error) {
	return d.viewAttemptsInternal(prod.Use(gaelogger.Use(c)), req)
}

func (*DungeonMaster) viewAttemptsInternal(c context.Context, req *ViewAttemptsReq) (*display.Data, error) {
	// TODO(iannucci): restrict this against current executions (or filter by
	// attempts the current execution is allowed to see).
	ds := datastore.Get(c)

	q := &model.Quest{ID: req.QuestID}
	if err := ds.Get(q); err != nil {
		return nil, fmt.Errorf("no such quest")
	}

	atmpts, err := q.GetAttempts(c)
	ret := &display.Data{}
	ret.Attempts = make(display.AttemptSlice, len(atmpts))
	for i, a := range atmpts {
		ret.Attempts[i] = a.ToDisplay()
	}
	sort.Sort(ret.Attempts)
	return ret, err
}

func init() {
	DungeonMasterMethodInfo["ViewAttempts"] = &endpoints.MethodInfo{
		Name:       "quests.attempts.list",
		HTTPMethod: "GET",
		Path:       "quests/{QuestID}/attempts",
		Desc:       "Lists the attempts of a Quest",
	}
}

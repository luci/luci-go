// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"fmt"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/gae/impl/prod"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/gaelogger"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/stringset"
	"golang.org/x/net/context"
)

// EnsureQuestsReq is the request for the EnsureQuests endpoint. It allows the
// specification of one or more Quests by their descriptor.
type EnsureQuestsReq struct {
	QuestDescriptors []*model.QuestDescriptor
}

// EnsureQuestsRsp is the response from the EnsureQuests endpoint. It lists the
// Quest IDs corresponding to each QuestDescriptor in the request.
type EnsureQuestsRsp struct {
	QuestIDs []string
}

// EnsureQuests allows a client to populate Quests by their descriptors. This
// is required before any Attempt is ensured for a Quest.
func (d *DungeonMaster) EnsureQuests(c context.Context, req *EnsureQuestsReq) (rsp *EnsureQuestsRsp, err error) {
	return d.ensureQuestsInternal(prod.Use(gaelogger.Use(c)), req)
}

func (*DungeonMaster) ensureQuestsInternal(c context.Context, req *EnsureQuestsReq) (*EnsureQuestsRsp, error) {
	questIDs := stringset.New(len(req.QuestDescriptors))
	distroNames := stringset.New(1)
	distros := make([]*model.Distributor, 0, len(req.QuestDescriptors))
	quests := make([]*model.Quest, 0, len(req.QuestDescriptors))
	rsp := &EnsureQuestsRsp{QuestIDs: make([]string, len(req.QuestDescriptors))}
	for i, q := range req.QuestDescriptors {
		qst, err := q.NewQuest(c)
		if err != nil {
			return nil, err
		}
		rsp.QuestIDs[i] = qst.ID
		if questIDs.Add(qst.ID) {
			quests = append(quests, qst)
		}
		if distroNames.Add(q.Distributor) {
			distros = append(distros, &model.Distributor{Name: q.Distributor})
		}
	}

	ds := datastore.Get(c)
	err := ds.GetMulti(distros)
	if err != nil {
		logging.Errorf(c, "error getting distributors: %s", err)
		return nil, fmt.Errorf("one or more unknown distributors")
	}

	err = ds.GetMulti(quests)
	if err == nil {
		return rsp, nil
	}

	merr, ok := err.(errors.MultiError)
	if !ok {
		return nil, err
	}

	toPut := []*model.Quest{}
	for i, e := range merr {
		if e == datastore.ErrNoSuchEntity {
			toPut = append(toPut, quests[i])
		} else if e != nil {
			return nil, e
		}
	}
	// TODO(iannucci): parallelly put these in tumble transactions with mutations
	// to create index/search entities.
	return rsp, ds.PutMulti(toPut)
}

func init() {
	DungeonMasterMethodInfo["EnsureQuests"] = &endpoints.MethodInfo{
		Name:       "quests.insert",
		HTTPMethod: "PUT",
		Path:       "quests",
		Desc:       "Ensures the existence of one or more Quests",
	}
}

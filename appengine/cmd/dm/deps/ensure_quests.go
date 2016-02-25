// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/stringset"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

func (d *deps) EnsureQuests(c context.Context, req *dm.EnsureQuestsReq) (*dm.EnsureQuestsRsp, error) {
	distroNames := stringset.New(1)
	questIDs := stringset.New(len(req.ToEnsure))
	quests := make([]*model.Quest, 0, len(req.ToEnsure))
	rsp := &dm.EnsureQuestsRsp{QuestIds: make([]*dm.Quest_ID, len(req.ToEnsure))}
	for i, desc := range req.ToEnsure {
		q, err := model.NewQuest(c, desc)
		if err != nil {
			return nil, grpcutil.Errf(codes.InvalidArgument, "bad descriptor %v", desc)
		}
		rsp.QuestIds[i] = &dm.Quest_ID{Id: q.ID}
		if questIDs.Add(q.ID) {
			quests = append(quests, q)
			distroNames.Add(q.Desc.DistributorConfigName)
		}
	}

	ds := datastore.Get(c)
	// TODO(riannucci): Validate that DistributorConfigName's all match current
	// luci-config.
	err := ds.GetMulti(quests)
	if err == nil {
		return rsp, nil
	}

	merr, ok := err.(errors.MultiError)
	if !ok {
		return nil, grpcutil.Errf(codes.Internal, "while trying to fetch quests: %s", err)
	}

	now := clock.Now(c)
	toPut := []*model.Quest{}
	failed := error(nil)
	for i, e := range merr {
		if e == datastore.ErrNoSuchEntity {
			quests[i].Created = now
			toPut = append(toPut, quests[i])
		} else if e != nil {
			logging.WithError(e).Errorf(c, "unknown error for quest %q", quests[i].ID)
			failed = grpcutil.Internal
		}
	}
	if failed != nil {
		return nil, failed
	}

	// TODO(iannucci): parallelly put these in tumble transactions with mutations
	// to create index/search entities.
	err = ds.PutMulti(toPut)
	if err != nil {
		logging.WithError(err).Errorf(c, "trying to put quests")
		err = grpcutil.Internal
	}
	return rsp, err
}

// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"google.golang.org/grpc/codes"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/mutate"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/stringset"
	"golang.org/x/net/context"
)

func (d *deps) AddDeps(c context.Context, req *dm.AddDepsReq) (rsp *dm.AddDepsRsp, err error) {
	if _, _, err = model.AuthenticateExecution(c, req.Auth); err != nil {
		return
	}

	err = req.Deps.Normalize()
	if err != nil {
		err = grpcutil.Errf(codes.InvalidArgument, "%s", err.Error())
		return
	}

	rsp = &dm.AddDepsRsp{}
	ds := datastore.Get(c)

	fwdDeps := model.FwdDepsFromFanout(c, req.Auth.Id.AttemptID(), req.Deps)

	// 1. Maybe all the dependencies already exist? If they do, then they must
	// all be completed, or we wouldn't be running the job right now.
	if ds.GetMulti(fwdDeps) == nil {
		return
	}

	// 2. Maybe all the target attempts are completed?
	targetQuests := stringset.New(len(req.Deps.To))
	atmpts := make([]*model.Attempt, len(fwdDeps))
	for i, dep := range fwdDeps {
		atmpts[i] = &model.Attempt{ID: dep.Dependee}
		targetQuests.Add(dep.Dependee.Quest)
	}
	_ = ds.GetMulti(atmpts)
	allFinished := true
	for _, a := range atmpts {
		if a == nil || a.State != dm.Attempt_Finished {
			allFinished = false
			break
		}
	}
	if allFinished {
		err = tumbleNow(c, &mutate.AddFinishedDeps{Auth: req.Auth, ToAdd: req.Deps})
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
			for _, e := range merr {
				if e == nil {
					continue
				}
				if e == datastore.ErrNoSuchEntity {
					logging.Infof(c, "user requested quest that DNE")
					err = grpcutil.Errf(codes.InvalidArgument, "one or more quests does not exist")
				} else {
					logging.WithError(e).Errorf(c, "error verifying quests")
					err = grpcutil.Errf(codes.Internal, "error verifying quests")
				}
				break
			}
		} else {
			logging.WithError(err).Errorf(c, "error in GetMulti")
			err = grpcutil.Errf(codes.Internal, "error verifying quests")
		}
	} else {
		rsp.ShouldHalt = true
		err = tumbleNow(c, &mutate.AddDeps{Auth: req.Auth, ToAdd: req.Deps})
	}
	return
}

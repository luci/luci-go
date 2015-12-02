// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	crand "crypto/rand" // TODO(iannucci): mock this for testing
	"encoding/hex"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/mathrand"
	"golang.org/x/net/context"
)

// ExecutionInfo is the structure returned from this temporary ClaimExecution
// API that includes he execution id and it's secret key.
type ExecutionInfo struct {
	ID  uint32
	Key []byte
}

// ClaimExecutionRsp is the response from ClaimExecution.
type ClaimExecutionRsp struct {
	Quest     *model.Quest
	Attempt   *display.Attempt
	Execution *ExecutionInfo

	NoneAvailable bool
}

const claimRetries = 8

// ClaimExecution is a temporary api which searches for and transactionally
// claims an execution for an Attempt whose state is NeedsExecution.
func (d *DungeonMaster) ClaimExecution(c context.Context) (rsp *ClaimExecutionRsp, err error) {
	if c, err = d.Use(c, MethodInfo["ClaimExecution"]); err != nil {
		return
	}

	// TODO(iannucci): stuff below
	// It's temporary!! Don't worry about it! :D
	//   Use of GET to mutate state
	//   No 3-way handshake means lost executions
	ds := datastore.Get(c)
	l := logging.Get(c)

	exKeyBytes := make([]byte, 32)
	_, err = crand.Read(exKeyBytes)
	if err != nil {
		return
	}
	rsp = &ClaimExecutionRsp{
		Execution: &ExecutionInfo{Key: exKeyBytes}}

	for attempts := 0; attempts < claimRetries; attempts++ {
		q := datastore.NewQuery("Attempt").Eq("State", types.NeedsExecution).Limit(32)

		if attempts < claimRetries-1 {
			prefixBytes := make([]byte, 2)
			_, err = crand.Read(prefixBytes)
			if err != nil {
				return
			}
			prefix := hex.EncodeToString(prefixBytes)
			l.Infof("Making query (prefix=%s)", prefix)
			q = q.Gte("__key__", ds.MakeKey("Attempt", prefix))
		} else {
			l.Infof("Making query (no prefix)")
		}

		as := []*model.Attempt{}
		err = ds.GetAll(q, &as)
		if err != nil {
			return
		}
		l.Infof("Got %d executions", len(as))
		if len(as) == 0 {
			continue
		}

		// Now find a random one which actually needs execution
		var aid types.AttemptID
		for _, i := range mathrand.Get(c).Perm(len(as)) {
			if as[i].State != types.NeedsExecution {
				continue
			}
			aid = as[i].AttemptID
		}
		if aid == (types.AttemptID{}) {
			continue
		}

		tryAgain := false

		// must Get outside of transaction.
		rsp.Quest = &model.Quest{ID: aid.QuestID}
		err = ds.Get(rsp.Quest)
		if err != nil {
			l.Errorf("error while trying to resolve quest: %s?", err)
			continue
		}

		err = ds.RunInTransaction(func(c context.Context) error {
			ds := datastore.Get(c)

			l.Infof("In TXN for %q", aid)
			// need to re-read this in the transaction to ensure it really does need
			// execution still :)
			a := &model.Attempt{AttemptID: aid}
			err := ds.Get(a)
			if err != nil {
				return err
			}
			if a.State != types.NeedsExecution {
				// oops, we picked a bad one, try again in the outer loop.
				tryAgain = true
				return nil
			}

			err = a.ChangeState(types.Executing)
			if err != nil {
				return err
			}

			a.CurExecution++

			ex := &model.Execution{
				ID:           a.CurExecution,
				Attempt:      ds.KeyForObj(a),
				ExecutionKey: rsp.Execution.Key,
			}
			rsp.Execution.ID = uint32(a.CurExecution)
			rsp.Attempt = a.ToDisplay()

			return ds.PutMulti([]interface{}{a, ex})
		}, nil)
		if tryAgain || err != nil {
			continue
		}
		return
	}

	rsp = &ClaimExecutionRsp{NoneAvailable: true}
	return
}

func init() {
	MethodInfo["ClaimExecution"] = &endpoints.MethodInfo{
		Name:       "executions.claim",
		HTTPMethod: "GET",
		Path:       "executions/claim",
		Desc:       "TEMP: Claim an execution id/key",
	}
}

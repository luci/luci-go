// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	crand "crypto/rand" // TODO(iannucci): mock this for testing
	"encoding/hex"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/mathrand"
	google_pb "github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
)

const claimRetries = 8

// ClaimExecution is a temporary api which searches for and transactionally
// claims an execution for an Attempt whose state is NeedsExecution.
func (d *deps) ClaimExecution(c context.Context, _ *google_pb.Empty) (rsp *dm.ClaimExecutionRsp, err error) {
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
	rsp = &dm.ClaimExecutionRsp{
		Auth: &dm.Execution_Auth{Token: exKeyBytes}}

	for attempts := 0; attempts < claimRetries; attempts++ {
		q := datastore.NewQuery("Attempt").Eq("State", dm.Attempt_NEEDS_EXECUTION).Limit(32)

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
		var aid dm.Attempt_ID
		for _, i := range mathrand.Get(c).Perm(len(as)) {
			if as[i].State != dm.Attempt_NEEDS_EXECUTION {
				continue
			}
			aid = as[i].ID
		}
		if aid == (dm.Attempt_ID{}) {
			continue
		}

		tryAgain := false

		err = ds.RunInTransaction(func(c context.Context) error {
			ds := datastore.Get(c)

			l.Infof("In TXN for %q", aid)
			// need to re-read this in the transaction to ensure it really does need
			// execution still :)
			a := &model.Attempt{ID: aid}
			err := ds.Get(a)
			if err != nil {
				return err
			}
			if a.State != dm.Attempt_NEEDS_EXECUTION {
				// oops, we picked a bad one, try again in the outer loop.
				tryAgain = true
				return nil
			}

			err = a.State.Evolve(dm.Attempt_EXECUTING)
			if err != nil {
				return err
			}

			a.CurExecution++

			ex := &model.Execution{
				ID:      a.CurExecution,
				Attempt: ds.KeyForObj(a),
				State:   dm.Execution_RUNNING,
				Token:   rsp.Auth.Token,
			}
			rsp.Auth.Id.Id = a.CurExecution
			rsp.Auth.Id.Attempt = a.ID.Id

			err = ds.PutMulti([]interface{}{a, ex})
			if err != nil {
				return err
			}

			qst := &model.Quest{ID: aid.Quest}
			err = datastore.GetNoTxn(c).Get(qst)
			if err != nil {
				return err
			}
			rsp.Quest = qst.ToProto()
			return nil
		}, nil)
		if tryAgain || err != nil {
			continue
		}
		return
	}

	rsp = nil
	return
}

// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"fmt"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/distributor"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	dm "github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
)

const numWorkers = 16

const maxTimeout = 55 * time.Second // GAE limit is 60s

type node struct {
	aid                 *dm.Attempt_ID
	depth               int64
	canSeeAttemptResult bool
}

func runSearchQuery(c context.Context, send func(*dm.Attempt_ID) error, s *dm.GraphQuery_Search) func() error {
	return func() error {
		logging.Errorf(c, "SearchQuery not implemented")
		return grpcutil.Errf(codes.Unimplemented, "GraphQuery.Search is not implemented")
	}
}

func isCtxErr(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func runAttemptListQuery(c context.Context, includeExpired bool, send func(*dm.Attempt_ID) error, al *dm.AttemptList) func() error {
	return func() error {
		for qst, anum := range al.To {
			if len(anum.Nums) == 0 {
				qry := model.QueryAttemptsForQuest(c, qst)
				if !includeExpired {
					qry = qry.Eq("ResultExpired", false)
				}
				err := datastore.Get(c).Run(qry, func(k *datastore.Key) error {
					aid := &dm.Attempt_ID{}
					if err := aid.SetDMEncoded(k.StringID()); err != nil {
						logging.WithError(err).Errorf(c, "Attempt_ID.SetDMEncoded returned an error with input: %q", k.StringID())
						panic(fmt.Errorf("in AttemptListQuery: %s", err))
					}
					return send(aid)
				})
				if err != nil {
					if !isCtxErr(err) {
						logging.WithError(err).Errorf(c, "in AttemptListQuery")
					}
					return err
				}
			} else {
				for _, num := range anum.Nums {
					if err := send(dm.NewAttemptID(qst, num)); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
}

func runAttemptRangeQuery(c context.Context, send func(*dm.Attempt_ID) error, ar *dm.GraphQuery_AttemptRange) func() error {
	return func() error {
		for i := ar.Low; i < ar.High; i++ {
			if err := send(dm.NewAttemptID(ar.Quest, i)); err != nil {
				return err
			}
		}
		return nil
	}
}

func questDataLoader(c context.Context, qid string, dst *dm.Quest) func() error {
	return func() error {
		qst := &model.Quest{ID: qid}
		if err := datastore.Get(c).Get(qst); err != nil {
			if err == datastore.ErrNoSuchEntity {
				dst.DNE = true
				dst.Partial = false
				return nil
			}
			logging.Fields{ek: err, "qid": qid}.Errorf(c, "while loading quest")
			return err
		}
		dst.Data = qst.DataProto()
		dst.Partial = false
		return nil
	}
}

func loadEdges(c context.Context, send func(*dm.Attempt_ID) error, typ string, base *datastore.Key, fan *dm.AttemptList, doSend bool) error {
	return datastore.Get(c).Run(datastore.NewQuery(typ).Ancestor(base), func(k *datastore.Key) error {
		if c.Err() != nil {
			return datastore.Stop
		}
		aid := &dm.Attempt_ID{}
		if err := aid.SetDMEncoded(k.StringID()); err != nil {
			logging.WithError(err).Errorf(c, "Attempt_ID.SetDMEncoded returned an error with input: %q", k.StringID())
			panic(fmt.Errorf("in AttemptListQuery: %s", err))
		}
		if fan != nil {
			fan.AddAIDs(aid)
		}
		if doSend {
			if err := send(aid); err != nil {
				return err
			}
		}
		return nil
	})
}

func maybeLoadInfoURL(c context.Context, reg distributor.Registry, dat *dm.Execution_Data_DistributorInfo) {
	d, _, err := reg.MakeDistributor(c, dat.ConfigName)
	if err != nil {
		logging.Fields{
			ek: err, "cfgName": dat.ConfigName,
		}.Errorf(c, "unable to load distributor")
	} else {
		dat.Url = d.InfoURL(distributor.Token(dat.Token))
	}
}

func loadExecutions(c context.Context, includeID, includeURL bool, atmpt *model.Attempt, akey *datastore.Key, numEx int64, dst *dm.Attempt) error {
	start := int64(atmpt.CurExecution) - numEx
	if start <= 0 {
		start = 1
	}
	toLoad := (int64(atmpt.CurExecution) - start) + 1
	if toLoad <= 0 {
		return nil
	}
	reg := distributor.GetRegistry(c)
	dst.Executions = make(map[uint32]*dm.Execution, toLoad)
	ds := datastore.Get(c)
	q := datastore.NewQuery("Execution").Ancestor(akey).Gte(
		"__key__", ds.MakeKey("Attempt", akey.StringID(), "Execution", start))
	return ds.Run(q, func(e *model.Execution) error {
		if c.Err() != nil {
			return datastore.Stop
		}
		p := e.ToProto(includeID)
		if includeURL {
			maybeLoadInfoURL(c, reg, p.Data.DistributorInfo)
		}
		dst.Executions[e.ID] = p
		return nil
	})
}

func attemptResultLoader(c context.Context, aid *dm.Attempt_ID, authedForResult bool, persistentState []byte, rsltSize uint32, lim *sizeLimit, akey *datastore.Key, auth *dm.Execution_Auth, dst *dm.Attempt) func() error {
	return func() error {
		ds := datastore.Get(c)
		if auth != nil && !authedForResult {
			// we need to prove that the currently authed execution depends on this
			// attempt.
			from := auth.Id.AttemptID().DMEncoded()
			to := aid.DMEncoded()
			fdepKey := ds.MakeKey("Attempt", from, "FwdDep", to)
			exist, err := ds.Exists(fdepKey)
			if err != nil {
				logging.Fields{ek: err, "key": fdepKey}.Errorf(c, "failed to determine if FwdDep exists")
				dst.Partial.Result = dm.Attempt_Partial_NOT_AUTHORIZED
				return err
			}
			if !exist.All() {
				dst.Partial.Result = dm.Attempt_Partial_NOT_AUTHORIZED
				return nil
			}
		}

		siz := rsltSize + uint32(len(persistentState))
		if !lim.PossiblyOK(siz) {
			dst.Partial.Result = dm.Attempt_Partial_DATA_SIZE_LIMIT
			logging.Infof(c, "skipping load of AttemptResult %s (size limit)", aid)
			return nil
		}
		r := &model.AttemptResult{Attempt: akey}
		if err := ds.Get(r); err != nil {
			logging.Fields{ek: err, "aid": aid}.Errorf(c, "failed to load AttemptResult")
			return err
		}
		if lim.Add(siz) {
			dst.Data.GetFinished().JsonResult = r.Data
			dst.Data.GetFinished().PersistentStateResult = persistentState
			dst.Partial.Result = dm.Attempt_Partial_LOADED
		} else {
			dst.Partial.Result = dm.Attempt_Partial_DATA_SIZE_LIMIT
			logging.Infof(c, "loaded AttemptResult %s, but hit size limit after", aid)
		}
		return nil
	}
}

func attemptLoader(c context.Context, req *dm.WalkGraphReq, aid *dm.Attempt_ID, authedForResult bool, lim *sizeLimit, dst *dm.Attempt, send func(aid *dm.Attempt_ID, fwd bool) error) func() error {
	return func() error {
		ds := datastore.Get(c)

		atmpt := &model.Attempt{ID: *aid}
		akey := ds.KeyForObj(atmpt)
		if err := ds.Get(atmpt); err != nil {
			if err == datastore.ErrNoSuchEntity {
				dst.DNE = true
				dst.Partial = nil
				return nil
			}
			return err
		}
		if !req.Include.ExpiredAttempts && atmpt.ResultExpired {
			return nil
		}

		persistentState := []byte(nil)
		if req.Include.AttemptData {
			dst.Data = atmpt.DataProto()
			dst.Partial.Data = false
			if fin := dst.Data.GetFinished(); fin != nil {
				// if we're including data and finished, we only add the persistentState
				// if we could see the attempt result. Save it off here, and restore it
				// in attemptResultLoader, only if we're able to load the actual result.
				//
				// This is done because for some jobs the persistentState is
				// almost-as-good as the actual result, and we want to prevent
				// false/accidental dependencies where a job is able to 'depend' on the
				// results without actually emitting a dependency on them.
				persistentState = fin.PersistentStateResult
				fin.PersistentStateResult = nil
			}
		}

		errChan := parallel.Run(0, func(ch chan<- func() error) {
			if req.Include.AttemptResult {
				if atmpt.State == dm.Attempt_FINISHED {
					ch <- attemptResultLoader(c, aid, authedForResult, persistentState, atmpt.ResultSize, lim, akey, req.Auth, dst)
				} else {
					dst.Partial.Result = dm.Attempt_Partial_LOADED
				}
			}

			if req.Include.NumExecutions > 0 {
				ch <- func() error {
					err := loadExecutions(c, req.Include.ObjectIds, req.Include.ExecutionInfoUrl, atmpt, akey, int64(req.Include.NumExecutions), dst)
					if err != nil {
						logging.Fields{ek: err, "aid": aid}.Errorf(c, "error loading executions")
					} else {
						dst.Partial.Executions = false
					}
					return err
				}
			}

			writeFwd := req.Include.FwdDeps
			walkFwd := (req.Mode.Direction == dm.WalkGraphReq_Mode_BOTH ||
				req.Mode.Direction == dm.WalkGraphReq_Mode_FORWARDS)
			loadFwd := writeFwd || walkFwd

			if loadFwd {
				isAuthed := req.Auth != nil && req.Auth.Id.AttemptID().Equals(aid)
				subSend := func(aid *dm.Attempt_ID) error {
					return send(aid, isAuthed)
				}
				if writeFwd {
					dst.FwdDeps = dm.NewAttemptList(nil)
				}
				ch <- func() error {
					err := loadEdges(c, subSend, "FwdDep", akey, dst.FwdDeps, walkFwd)
					if err == nil && dst.Partial != nil {
						dst.Partial.FwdDeps = false
					} else if err != nil {
						logging.Fields{"aid": aid, ek: err}.Errorf(c, "while loading FwdDeps")
					}
					return err
				}
			}

			writeBack := req.Include.BackDeps
			walkBack := (req.Mode.Direction == dm.WalkGraphReq_Mode_BOTH ||
				req.Mode.Direction == dm.WalkGraphReq_Mode_BACKWARDS)
			loadBack := writeBack || walkBack

			if loadBack {
				if writeBack {
					dst.BackDeps = dm.NewAttemptList(nil)
				}
				subSend := func(aid *dm.Attempt_ID) error {
					return send(aid, false)
				}
				bdg := &model.BackDepGroup{Dependee: atmpt.ID}
				bdgKey := ds.KeyForObj(bdg)
				ch <- func() error {
					err := loadEdges(c, subSend, "BackDep", bdgKey, dst.BackDeps, walkBack)
					if err == nil && dst.Partial != nil {
						dst.Partial.BackDeps = false
					} else if err != nil {
						logging.Fields{"aid": aid, ek: err}.Errorf(c, "while loading FwdDeps")
					}
					return err
				}
			}
		})
		hadErr := false
		for err := range errChan {
			if err == nil {
				continue
			}
			if err != nil {
				hadErr = true
			}
		}
		dst.NormalizePartial()
		if hadErr {
			return fmt.Errorf("had one or more errors loading attempt")
		}
		return nil
	}
}

// WalkGraph does all the graph walking in parallel. Theoretically, the
// goroutines look like:
//
//   main:
//     WorkPool(MaxWorkers+1) as pool
//       jobBuf.add(FanOutIn(Query1->nodeChan ... QueryN->nodeChan))
//       runningWorkers = 0
//       while runningWorkers > 0 or jobBuf is not empty:
//         select {
//           pool <- jobBuf.pop(): // only if jobBuf !empty, otherwise skip this send
//             runningWorkers++
//           node = <- nodeChan:
//             if node == finishedJob {
//               runningWorkers--
//             } else {
//               if node mentions new quest:
//                 jobBuf.add(questDataLoader()->nodeChan)
//               if node mentions new attempt:
//                 if node.depth < MaxDepth:
//                   jobBuf.add(attemptLoader(node.depth+1)->nodeChan)
//             }
//         }
//
//   attemptLoader:
//     a = get_attempt()
//     if a exists {
//       FanOutIn(
//         maybeLoadAttemptResult,
//         maybeLoadExecutions,
//         maybeLoadFwdDeps, // sends to nodeChan if walking direction Fwd|Both
//         maybeLoadBackDeps // sends to nodeChan if walking direction Back|Both
//       )
//     }
func (d *deps) WalkGraph(c context.Context, req *dm.WalkGraphReq) (rsp *dm.GraphData, err error) {
	if req.Auth != nil {
		logging.Fields{"execution": req.Auth.Id}.Debugf(c, "on behalf of")
	} else {
		logging.Debugf(c, "for user")
	}

	cncl := (func())(nil)
	timeoutProto := req.Limit.MaxTime
	timeout := timeoutProto.Duration() // .Duration on nil is OK
	if timeoutProto == nil || timeout > maxTimeout {
		timeout = maxTimeout
	}
	c, cncl = clock.WithTimeout(c, timeout)
	defer cncl()

	// nodeChan recieves attempt nodes to process. If it recieves the
	// `finishedJob` sentinel node, that indicates that an outstanding worker is
	// finished.
	nodeChan := make(chan *node, numWorkers)
	defer close(nodeChan)

	sendNodeAuthed := func(depth int64) func(*dm.Attempt_ID, bool) error {
		return func(aid *dm.Attempt_ID, isAuthed bool) error {
			select {
			case nodeChan <- &node{aid: aid, depth: depth, canSeeAttemptResult: isAuthed}:
				return nil
			case <-c.Done():
				return c.Err()
			}
		}
	}

	sendNode := func(depth int64) func(*dm.Attempt_ID) error {
		return func(aid *dm.Attempt_ID) error {
			select {
			case nodeChan <- &node{aid: aid, depth: depth}:
				return nil
			case <-c.Done():
				return c.Err()
			}
		}
	}

	buf := parallel.Buffer{}
	defer buf.Close()
	buf.Maximum = numWorkers + 1 // +1 job for queries
	buf.Sustained = numWorkers
	buf.SetFIFO(!req.Mode.Dfs)

	allErrs := make(chan error, numWorkers)
	outstandingJobs := 0

	addJob := func(f func() error) {
		if c.Err() != nil { // we're done adding work, ignore this job
			return
		}
		outstandingJobs++
		buf.WorkC() <- parallel.WorkItem{F: f, ErrC: allErrs}
	}

	addJob(func() error {
		return parallel.FanOutIn(func(pool chan<- func() error) {
			q := req.Query
			snd := sendNode(0)
			if q.AttemptList != nil {
				pool <- runAttemptListQuery(c, req.Include.ExpiredAttempts, snd, q.AttemptList)
			}
			for _, rng := range q.AttemptRange {
				pool <- runAttemptRangeQuery(c, snd, rng)
			}
			for _, srch := range q.Search {
				pool <- runSearchQuery(c, snd, srch)
			}
		})
	})

	dataLimit := &sizeLimit{0, req.Limit.MaxDataSize}
	rsp = &dm.GraphData{Quests: map[string]*dm.Quest{}}
	// main graph walk processing loop
	for outstandingJobs > 0 || len(nodeChan) > 0 {
		select {
		case err := <-allErrs:
			outstandingJobs--
			if err == nil {
				break
			}
			if !isCtxErr(err) {
				rsp.HadErrors = true
			}
			// assume that contextualized logging already happened

		case n := <-nodeChan:
			qst, ok := rsp.GetQuest(n.aid.Quest)
			if !ok {
				if !req.Include.ObjectIds {
					qst.Id = nil
				}
				if req.Include.QuestData {
					addJob(questDataLoader(c, n.aid.Quest, qst))
				}
			}
			if _, ok := qst.Attempts[n.aid.Id]; !ok {
				atmpt := &dm.Attempt{Partial: &dm.Attempt_Partial{
					Data:       req.Include.AttemptData,
					Executions: req.Include.NumExecutions != 0,
					FwdDeps:    req.Include.FwdDeps,
					BackDeps:   req.Include.BackDeps,
				}}
				if req.Include.AttemptResult {
					atmpt.Partial.Result = dm.Attempt_Partial_NOT_LOADED
				}

				atmpt.NormalizePartial() // in case they're all false
				if req.Include.ObjectIds {
					atmpt.Id = n.aid
				}
				qst.Attempts[n.aid.Id] = atmpt
				if req.Limit.MaxDepth == -1 || n.depth < req.Limit.MaxDepth {
					addJob(attemptLoader(c, req, n.aid, n.canSeeAttemptResult, dataLimit, atmpt,
						sendNodeAuthed(n.depth+1)))
				}
			}
			// otherwise, we've dealt with this attempt before, so ignore it.
		}
	}

	if c.Err() != nil {
		rsp.HadMore = true
	}
	return
}

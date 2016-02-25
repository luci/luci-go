// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deps

import (
	"fmt"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

const numWorkers = 16

const maxTimeout = 55 * time.Second // GAE limit is 60s

type node struct {
	aid   *dm.Attempt_ID
	depth int64
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

func runAttemptListQuery(c context.Context, includeExpired bool, send func(*dm.Attempt_ID) error, al *dm.GraphQuery_AttemptList) func() error {
	return func() error {
		for qst, anum := range al.Attempt.To {
			if len(anum.Nums) == 0 {
				qry := model.QueryAttemptsForQuest(c, qst)
				if !includeExpired {
					qry = qry.Eq("Expired", false)
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

func runQuery(c context.Context, includeExpired bool, send func(*dm.Attempt_ID) error, q *dm.GraphQuery) func() error {
	switch q := q.Query.(type) {
	case nil:
		return func() error {
			return grpcutil.Errf(codes.InvalidArgument, "empty query")
		}
	case *dm.GraphQuery_Search_:
		return runSearchQuery(c, send, q.Search)
	case *dm.GraphQuery_AttemptList_:
		return runAttemptListQuery(c, includeExpired, send, q.AttemptList)
	case *dm.GraphQuery_AttemptRange_:
		return runAttemptRangeQuery(c, send, q.AttemptRange)
	default:
		panic(fmt.Errorf("unknown query type: %T", q))
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

func loadEdges(c context.Context, send func(*dm.Attempt_ID) error, typ string, base *datastore.Key, fan *dm.AttemptFanout, doSend bool) error {
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

func loadExecutions(c context.Context, includeID bool, atmpt *model.Attempt, akey *datastore.Key, numEx int64, dst *dm.Attempt) error {
	start := int64(atmpt.CurExecution) - numEx
	if start <= 0 {
		start = 1
	}
	toLoad := (int64(atmpt.CurExecution) - start) + 1
	if toLoad <= 0 {
		return nil
	}
	dst.Executions = make(map[uint32]*dm.Execution, toLoad)
	ds := datastore.Get(c)
	q := datastore.NewQuery("Execution").Ancestor(akey).Gte(
		"__key__", ds.MakeKey("Attempt", akey.StringID(), "Execution", start))
	return ds.Run(q, func(e *model.Execution) error {
		if c.Err() != nil {
			return datastore.Stop
		}
		dst.Executions[e.ID] = e.ToProto(includeID)
		return nil
	})
}

func attemptResultLoader(c context.Context, aid *dm.Attempt_ID, akey *datastore.Key, auth *dm.Execution_Auth, dst *dm.Attempt) func() error {
	return func() error {
		ds := datastore.Get(c)
		if auth != nil {
			from := auth.Id.AttemptID().DMEncoded()
			to := aid.DMEncoded()
			fdepKey := ds.MakeKey("Attempt", from, "FwdDep", to)
			exist, err := ds.Exists(fdepKey)
			if err != nil {
				logging.Fields{ek: err, "key": fdepKey}.Errorf(c, "failed to determine if FwdDep exists")
				return err
			}
			if !exist {
				dst.Partial.Result = false
				return nil
			}
		}

		r := &model.AttemptResult{Attempt: akey}
		if err := ds.Get(r); err != nil {
			logging.Fields{ek: err, "aid": aid}.Errorf(c, "failed to load AttemptResult")
			return err
		}
		dst.Data.GetFinished().JsonResult = r.Data
		dst.Partial.Result = false
		return nil
	}
}

func attemptLoader(c context.Context, req *dm.WalkGraphReq, aid *dm.Attempt_ID, dst *dm.Attempt, send func(*dm.Attempt_ID) error) func() error {
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
		if !req.Include.ExpiredAttempts && atmpt.Expired {
			return nil
		}
		if req.Include.AttemptData {
			dst.Data = atmpt.DataProto()
			dst.Partial.Data = false
		}

		errChan := parallel.Run(0, func(ch chan<- func() error) {
			if req.Include.AttemptResult {
				if atmpt.State == dm.Attempt_Finished {
					ch <- attemptResultLoader(c, aid, akey, req.Auth, dst)
				} else {
					dst.Partial.Result = false
				}
			}

			if req.Include.NumExecutions > 0 {
				ch <- func() error {
					err := loadExecutions(c, req.Include.ObjectIds, atmpt, akey, int64(req.Include.NumExecutions), dst)
					if err != nil {
						logging.Fields{ek: err, "aid": aid}.Errorf(c, "error loading executions")
					} else {
						dst.Partial.Executions = false
					}
					return err
				}
			}

			writeFwd := req.Include.FwdDeps
			walkFwd := (req.Mode.Direction == dm.WalkGraphReq_Mode_Both ||
				req.Mode.Direction == dm.WalkGraphReq_Mode_Forwards)
			loadFwd := writeFwd || walkFwd

			if loadFwd {
				if writeFwd {
					dst.FwdDeps = dm.NewAttemptFanout(nil)
				}
				ch <- func() error {
					err := loadEdges(c, send, "FwdDep", akey, dst.FwdDeps, walkFwd)
					if err == nil && dst.Partial != nil {
						dst.Partial.FwdDeps = false
					} else if err != nil {
						logging.Fields{"aid": aid, ek: err}.Errorf(c, "while loading FwdDeps")
					}
					return err
				}
			}

			writeBack := req.Include.BackDeps
			walkBack := (req.Mode.Direction == dm.WalkGraphReq_Mode_Both ||
				req.Mode.Direction == dm.WalkGraphReq_Mode_Backwards)
			loadBack := writeBack || walkBack

			if loadBack {
				if writeBack {
					dst.BackDeps = dm.NewAttemptFanout(nil)
				}
				bdg := &model.BackDepGroup{Dependee: atmpt.ID}
				bdgKey := ds.KeyForObj(bdg)
				ch <- func() error {
					err := loadEdges(c, send, "BackDep", bdgKey, dst.BackDeps, walkBack)
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
	if err = req.Normalize(); err != nil {
		return nil, grpcutil.Errf(codes.InvalidArgument, err.Error())
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
			for _, q := range req.Queries {
				select {
				case pool <- runQuery(c, req.Include.ExpiredAttempts, sendNode(0), q):
				case <-c.Done():
					return
				}
			}
		})
	})

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
			qst, ok := rsp.Quests[n.aid.Quest]
			if !ok {
				qst = &dm.Quest{
					Attempts: map[uint32]*dm.Attempt{},
					Partial:  req.Include.QuestData,
				}
				if req.Include.ObjectIds {
					qst.Id = &dm.Quest_ID{Id: n.aid.Quest}
				}
				rsp.Quests[n.aid.Quest] = qst
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
					Result:     req.Include.AttemptResult,
				}}
				atmpt.NormalizePartial() // in case they're all false
				if req.Include.ObjectIds {
					atmpt.Id = n.aid
				}
				qst.Attempts[n.aid.Id] = atmpt
				if req.Limit.MaxDepth == -1 || n.depth < req.Limit.MaxDepth {
					addJob(attemptLoader(c, req, n.aid, atmpt, sendNode(n.depth+1)))
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

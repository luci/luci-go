// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/dumper"
	"github.com/luci/luci-go/appengine/cmd/dm/distributor/fake"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/mutate"
	"github.com/luci/luci-go/appengine/tumble"
	dm "github.com/luci/luci-go/common/api/dm/service/v1"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func testSetup() (ttest *tumble.Testing, c context.Context, dist *fake.Distributor, s testDepsServer) {
	ttest, c, dist, reg := fake.Setup(mutate.FinishExecutionFn)
	s = testDepsServer{newDecoratedDeps(reg)}
	return
}

type testDepsServer struct {
	dm.DepsServer
}

func (s testDepsServer) ensureQuest(c context.Context, name string, aids ...uint32) string {
	desc := fake.QuestDesc(name)
	q := model.NewQuest(c, desc)
	qsts, err := s.EnsureGraphData(c, &dm.EnsureGraphDataReq{
		Quest:    []*dm.Quest_Desc{desc},
		Attempts: dm.NewAttemptList(map[string][]uint32{q.ID: aids}),
	})
	So(err, ShouldBeNil)
	for qid := range qsts.Result.Quests {
		So(qid, ShouldEqual, q.ID)
		return qid
	}
	panic("impossible")
}

func dumpDatastore(c context.Context) {
	ds := datastore.Get(c)
	snap := ds.Testable().TakeIndexSnapshot()
	ds.Testable().CatchupIndexes()
	defer ds.Testable().SetIndexSnapshot(snap)

	fmt.Println("dumping datastore")
	dumper.Config{
		PropFilters: dumper.PropFilterMap{
			{"Quest", "Desc"}: func(prop datastore.Property) string {
				desc := &dm.Quest_Desc{}
				if err := proto.Unmarshal(prop.Value().([]byte), desc); err != nil {
					panic(err)
				}
				return desc.String()
			},
			{"Attempt", "State"}: func(prop datastore.Property) string {
				return dm.Attempt_State(int32(prop.Value().(int64))).String()
			},
			{"Execution", "State"}: func(prop datastore.Property) string {
				return dm.Execution_State(int32(prop.Value().(int64))).String()
			},
		},
	}.Query(c, nil)
}

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeStore struct {
	cells         []types.Cell
	defaultTarget types.Target
}

func (s *fakeStore) Register(types.Metric)       {}
func (s *fakeStore) Unregister(types.Metric)     {}
func (s *fakeStore) DefaultTarget() types.Target { return s.defaultTarget }
func (s *fakeStore) Get(context.Context, types.Metric, time.Time, []interface{}) (interface{}, error) {
	return nil, nil
}
func (s *fakeStore) Set(context.Context, types.Metric, time.Time, []interface{}, interface{}) error {
	return nil
}
func (s *fakeStore) Incr(context.Context, types.Metric, time.Time, []interface{}, interface{}) error {
	return nil
}
func (s *fakeStore) ModifyMulti(ctx context.Context, mods []store.Modification) error { return nil }
func (s *fakeStore) GetAll(context.Context) []types.Cell                              { return s.cells }
func (s *fakeStore) ResetForUnittest()                                                {}

type fakeMonitor struct {
	chunkSize int
	cells     [][]types.Cell
}

func (m *fakeMonitor) ChunkSize() int {
	return m.chunkSize
}

func (m *fakeMonitor) Send(cells []types.Cell) error {
	m.cells = append(m.cells, cells)
	return nil
}

func TestFlush(t *testing.T) {
	ctx := context.Background()

	defaultTarget := (*target.Task)(&pb.Task{
		ServiceName: proto.String("test"),
	})

	Convey("Sends a metric", t, func() {
		s := fakeStore{
			cells: []types.Cell{
				{
					types.MetricInfo{
						Name:      "foo",
						Fields:    []field.Field{},
						ValueType: types.StringType,
					},
					types.CellData{
						FieldVals: []interface{}{},
						ResetTime: time.Unix(1234, 1000),
						Value:     "bar",
					},
				},
			},
			defaultTarget: defaultTarget,
		}
		globalStore = &s

		m := fakeMonitor{
			chunkSize: 42,
			cells:     [][]types.Cell{},
		}
		globalMonitor = &m

		So(Flush(ctx), ShouldBeNil)

		So(len(m.cells), ShouldEqual, 1)
		So(len(m.cells[0]), ShouldEqual, 1)
		So(m.cells[0][0], ShouldResemble, types.Cell{
			types.MetricInfo{
				Name:      "foo",
				Fields:    []field.Field{},
				ValueType: types.StringType,
			},
			types.CellData{
				FieldVals: []interface{}{},
				ResetTime: time.Unix(1234, 1000),
				Value:     "bar",
			},
		})
	})

	Convey("Splits up ChunkSize metrics", t, func() {
		s := fakeStore{
			cells:         make([]types.Cell, 43),
			defaultTarget: defaultTarget,
		}
		globalStore = &s

		m := fakeMonitor{
			chunkSize: 42,
			cells:     [][]types.Cell{},
		}
		globalMonitor = &m

		for i := 0; i < 43; i++ {
			s.cells[i] = types.Cell{
				types.MetricInfo{
					Name:      "foo",
					Fields:    []field.Field{},
					ValueType: types.StringType,
				},
				types.CellData{
					FieldVals: []interface{}{},
					ResetTime: time.Unix(1234, 1000),
					Value:     "bar",
				},
			}
		}

		So(Flush(ctx), ShouldBeNil)

		So(len(m.cells), ShouldEqual, 2)
		So(len(m.cells[0]), ShouldEqual, 42)
		So(len(m.cells[1]), ShouldEqual, 1)
	})

	Convey("Doesn't split metrics when ChunkSize is 0", t, func() {
		s := fakeStore{
			cells:         make([]types.Cell, 43),
			defaultTarget: defaultTarget,
		}
		globalStore = &s

		m := fakeMonitor{
			chunkSize: 0,
			cells:     [][]types.Cell{},
		}
		globalMonitor = &m

		for i := 0; i < 43; i++ {
			s.cells[i] = types.Cell{
				types.MetricInfo{
					Name:      "foo",
					Fields:    []field.Field{},
					ValueType: types.StringType,
				},
				types.CellData{
					FieldVals: []interface{}{},
					ResetTime: time.Unix(1234, 1000),
					Value:     "bar",
				},
			}
		}

		So(Flush(ctx), ShouldBeNil)

		So(len(m.cells), ShouldEqual, 1)
		So(len(m.cells[0]), ShouldEqual, 43)
	})

	Convey("No Monitor configured", t, func() {
		globalMonitor = nil

		So(Flush(ctx), ShouldNotBeNil)
	})

	Convey("Auto flush works", t, func() {
		start := time.Unix(1454561232, 0)
		ctx, tc := testclock.UseTime(ctx, start)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			tc.Add(d)
		})

		moments := make(chan int)
		flusher := autoFlusher{
			flush: func(c context.Context) error {
				select {
				case <-c.Done():
				case moments <- int(clock.Now(ctx).Sub(start).Seconds()):
				}
				return nil
			},
		}

		flusher.start(ctx, time.Second)

		// Each 'flush' gets blocked on sending into 'moments'. Once unblocked, it
		// advances timer by 'interval' sec (1 sec in the test).
		So(<-moments, ShouldEqual, 1)
		So(<-moments, ShouldEqual, 2)
		// and so on ...

		// Doesn't timeout => works.
		flusher.stop()
	})
}

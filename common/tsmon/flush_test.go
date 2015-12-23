// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeStore struct {
	cells []types.Cell
}

func (s *fakeStore) Register(types.Metric) (store.MetricHandle, error) { return nil, nil }
func (s *fakeStore) Unregister(store.MetricHandle)                     {}
func (s *fakeStore) Get(context.Context, store.MetricHandle, time.Time, []interface{}) (interface{}, error) {
	return nil, nil
}
func (s *fakeStore) Set(context.Context, store.MetricHandle, time.Time, []interface{}, interface{}) error {
	return nil
}
func (s *fakeStore) Incr(context.Context, store.MetricHandle, time.Time, []interface{}, interface{}) error {
	return nil
}
func (s *fakeStore) GetAll(context.Context) []types.Cell { return s.cells }
func (s *fakeStore) ResetForUnittest()                   {}

type fakeMonitor struct {
	chunkSize int
	cells     [][]types.Cell
}

func (m *fakeMonitor) ChunkSize() int {
	return m.chunkSize
}

func (m *fakeMonitor) Send(cells []types.Cell, t types.Target) error {
	m.cells = append(m.cells, cells)
	return nil
}

func TestFlush(t *testing.T) {
	ctx := context.Background()

	Target = (*target.Task)(&pb.Task{
		ServiceName: proto.String("test"),
	})

	Convey("Sends a metric", t, func() {
		s := fakeStore{
			cells: []types.Cell{
				{
					types.MetricInfo{
						MetricName: "foo",
						Fields:     []field.Field{},
						ValueType:  types.StringType,
					},
					types.CellData{
						FieldVals: []interface{}{},
						ResetTime: time.Unix(1234, 1000),
						Value:     "bar",
					},
				},
			},
		}
		Store = &s

		m := fakeMonitor{
			chunkSize: 42,
			cells:     [][]types.Cell{},
		}
		Monitor = &m

		Flush(ctx)

		So(len(m.cells), ShouldEqual, 1)
		So(len(m.cells[0]), ShouldEqual, 1)
		So(m.cells[0][0], ShouldResemble, types.Cell{
			types.MetricInfo{
				MetricName: "foo",
				Fields:     []field.Field{},
				ValueType:  types.StringType,
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
			cells: make([]types.Cell, 43),
		}
		Store = &s

		m := fakeMonitor{
			chunkSize: 42,
			cells:     [][]types.Cell{},
		}
		Monitor = &m

		for i := 0; i < 43; i++ {
			s.cells[i] = types.Cell{
				types.MetricInfo{
					MetricName: "foo",
					Fields:     []field.Field{},
					ValueType:  types.StringType,
				},
				types.CellData{
					FieldVals: []interface{}{},
					ResetTime: time.Unix(1234, 1000),
					Value:     "bar",
				},
			}
		}

		Flush(ctx)

		So(len(m.cells), ShouldEqual, 2)
		So(len(m.cells[0]), ShouldEqual, 42)
		So(len(m.cells[1]), ShouldEqual, 1)
	})

	Convey("Doesn't split metrics when ChunkSize is 0", t, func() {
		s := fakeStore{
			cells: make([]types.Cell, 43),
		}
		Store = &s

		m := fakeMonitor{
			chunkSize: 0,
			cells:     [][]types.Cell{},
		}
		Monitor = &m

		for i := 0; i < 43; i++ {
			s.cells[i] = types.Cell{
				types.MetricInfo{
					MetricName: "foo",
					Fields:     []field.Field{},
					ValueType:  types.StringType,
				},
				types.CellData{
					FieldVals: []interface{}{},
					ResetTime: time.Unix(1234, 1000),
					Value:     "bar",
				},
			}
		}

		Flush(ctx)

		So(len(m.cells), ShouldEqual, 1)
		So(len(m.cells[0]), ShouldEqual, 43)
	})

	Convey("No Monitor configured", t, func() {
		Monitor = nil

		err := Flush(ctx)
		So(err, ShouldNotBeNil)
	})
}

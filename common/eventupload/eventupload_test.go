// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eventupload

import (
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/tsmon"

	"infra/libs/eventupload/testdata"

	. "github.com/smartystreets/goconvey/convey"
)

// MockUploader is an EventUploader that can be used for testing.
type MockUploader chan []fakeEvent

// Put sends events on the MockUploader.
func (u MockUploader) Put(ctx context.Context, src interface{}) error {
	srcs := src.([]interface{})
	var fes []fakeEvent
	for _, fe := range srcs {
		fes = append(fes, fe.(fakeEvent))
	}
	u <- fes
	return nil
}

type fakeEvent struct{}

// ShouldHavePut verifies that Put was called with the expected set of events.
func ShouldHavePut(actual interface{}, expected ...interface{}) string {
	u := actual.(MockUploader)

	select {
	case got := <-u:
		return ShouldResemble([]interface{}{got}, expected)
	case <-time.After(50 * time.Millisecond):
		return "timed out waiting for upload"
	}
}

func TestMetric(t *testing.T) {
	t.Parallel()
	ctx := tsmon.WithState(context.Background(), tsmon.NewState())
	u := Uploader{}
	u.UploadsMetricName = "fakeCounter"
	Convey("Test metric creation", t, func() {
		Convey("Expect uploads metric was created", func() {
			_ = u.getCounter(ctx) // To actually create the metric
			So(u.uploads.Info().Name, ShouldEqual, "fakeCounter")
		})
	})
}

func TestClose(t *testing.T) {
	t.Parallel()

	Convey("Test Close", t, func() {
		u := make(MockUploader, 1)
		bu, err := NewBatchUploader(context.Background(), u, make(chan time.Time))
		if err != nil {
			t.Fatal(err)
		}

		closed := false
		defer func() {
			if !closed {
				bu.Close()
			}
		}()

		Convey("Expect Stage to add event to pending queue", func() {
			bu.Stage(fakeEvent{})
			So(bu.pending, ShouldHaveLength, 1)
		})

		Convey("Expect Close to flush pending queue", func() {
			bu.Stage(fakeEvent{})
			bu.Close()
			closed = true
			So(bu.pending, ShouldHaveLength, 0)
			So(u, ShouldHavePut, []fakeEvent{{}})
			So(bu.isClosed(), ShouldBeTrue)
			So(func() { bu.Stage(fakeEvent{}) }, ShouldPanic)
		})
	})
}

func TestUpload(t *testing.T) {
	t.Parallel()

	Convey("Test Upload", t, func() {
		u := make(MockUploader, 1)
		tickc := make(chan time.Time)
		bu, err := NewBatchUploader(context.Background(), &u, tickc)
		if err != nil {
			t.Fatal(err)
		}
		defer bu.Close()

		bu.Stage(fakeEvent{})
		Convey("Expect Put to wait for tick to call upload", func() {
			So(u, ShouldHaveLength, 0)
			tickc <- time.Time{}
			So(u, ShouldHavePut, []fakeEvent{{}})
		})
	})
}

func TestSave(t *testing.T) {
	Convey("TestSave", t, func() {
		ts, err := ptypes.TimestampProto(time.Time{})
		if err != nil {
			t.Fatal("could not convert time to timestamp")
		}
		r := &Row{
			Message: &testdata.TestMessage{
				Name:      "testname",
				Timestamp: ts,
				Nested: &testdata.NestedTestMessage{
					Name: "nestedname",
				},
				RepeatedNested: []*testdata.NestedTestMessage{
					{Name: "repeated_one"},
					{Name: "repeated_two"},
				},
			},
			InsertID: "testid",
		}
		row, id, err := r.Save()
		So(row["name"], ShouldEqual, "testname")
		So(row["timestamp"].(time.Time), ShouldResemble, time.Time{})
		So(row["nested"].(map[string]bigquery.Value)["name"], ShouldEqual, "nestedname")
		So(row["repeated_nested"].([]interface{})[0].(map[string]bigquery.Value)["name"], ShouldEqual, "repeated_one")
		So(id, ShouldEqual, "testid")
		So(err, ShouldBeNil)
	})
}

func TestRowsFromSrc(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		desc    string
		src     interface{}
		wantLen int
	}{
		{
			desc:    "accepts pointers to structs which implement proto.Message",
			src:     &testdata.TestMessage{},
			wantLen: 1,
		},
		// TODO: when proto transition is complete, change to does not accept
		{
			desc:    "accepts structs",
			src:     testdata.TestMessage{},
			wantLen: 1,
		},
		// TODO: when proto transition is complete, change to does not accept
		{
			desc:    "accepts pointers to structs which do not implement proto.Message",
			src:     &fakeEvent{},
			wantLen: 1,
		},
		{
			desc:    "accepts slices of structs which implement proto.Message",
			src:     []*testdata.TestMessage{{}, {}},
			wantLen: 2,
		},
	}

	Convey("test rowsFromSrc()", t, func() {
		for _, tc := range tcs {
			Convey(tc.desc, func() {
				r, _ := rowsFromSrc(tc.src)
				So(r, ShouldHaveLength, tc.wantLen)
			})
		}
	})
}

func TestStage(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		desc    string
		src     interface{}
		wantLen int
	}{
		{
			desc:    "single event",
			src:     fakeEvent{},
			wantLen: 1,
		},
		{
			desc:    "slice of events",
			src:     []fakeEvent{{}, {}},
			wantLen: 2,
		},
	}

	Convey("Stage can accept single events and slices of events", t, func() {
		u := make(MockUploader, 1)
		bu, err := NewBatchUploader(context.Background(), &u, make(chan time.Time))
		if err != nil {
			t.Fatal(err)
		}
		defer bu.Close()

		for _, tc := range tcs {
			Convey(fmt.Sprintf("Test %s", tc.desc), func() {
				bu.Stage(tc.src)
				So(bu.pending, ShouldHaveLength, tc.wantLen)
				So(bu.pending[len(bu.pending)-1], ShouldHaveSameTypeAs, fakeEvent{})
			})
		}
	})
}

func TestBatch(t *testing.T) {
	t.Parallel()

	Convey("Test batch", t, func() {
		rowLimit := 2
		// TODO: when proto transition is complete, use *Rows, not ValueSavers
		rows := make([]bigquery.ValueSaver, 3)
		for i := 0; i < 3; i++ {
			rows[i] = &Row{}
		}

		want := [][]bigquery.ValueSaver{
			{
				&Row{},
				&Row{},
			},
			{
				&Row{},
			},
		}
		rowSets := batch(rows, rowLimit)
		So(rowSets, ShouldResemble, want)
	})
}

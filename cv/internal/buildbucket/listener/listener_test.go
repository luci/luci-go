// Copyright 2021 The LUCI Authors.
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

package bblistener

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"cloud.google.com/go/pubsub"

	v1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/retry/transient"
	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetBuildIDFromPubsubMessage(t *testing.T) {
	t.Parallel()
	Convey("works", t, func() {
		original := tryjob.MustBuildbucketID(successHost, 123456789)
		extracted, err := getBuildIDFromPubsubMessage(context.Background(), mockBuildNotification(original).GetData())
		So(err, ShouldBeNil)
		So(extracted, ShouldEqual, original)
	})
	Convey("no ID", t, func() {
		buildJSON, err := json.Marshal(&v1.LegacyApiBuildResponseMessage{Build: &v1.LegacyApiCommonBuildMessage{}})
		So(err, ShouldBeNil)
		message := &pubsub.Message{Data: buildJSON}
		eid, err := getBuildIDFromPubsubMessage(context.Background(), message.Data)
		So(err, ShouldErrLike, "missing build details")
		So(eid, ShouldEqual, tryjob.ExternalID(""))
	})
	Convey("no Build", t, func() {
		buildJSON, err := json.Marshal(&v1.LegacyApiBuildResponseMessage{})
		So(err, ShouldBeNil)
		message := &pubsub.Message{Data: buildJSON}
		eid, err := getBuildIDFromPubsubMessage(context.Background(), message.Data)
		So(err, ShouldErrLike, "missing build details")
		So(eid, ShouldEqual, tryjob.ExternalID(""))
	})
}

const (
	successHost   = "cr-buildbucket.example.com"
	permFailHost  = "perm-cr-buildbucket.example.com"
	transFailHost = "trans-cr-buildbucket.example.com"
)

func TestProcessNotificationBatch(t *testing.T) {
	t.Parallel()
	Convey("test processNotificationsBatch", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		tjNotifier := &testTryjobNotifier{
			ok:    make(stringset.Set),
			trans: make(stringset.Set),
			perm:  make(stringset.Set),
		}

		Convey("Successful", func() {
			Convey("Relevant", func() {
				eid := tryjob.MustBuildbucketID(successHost, 1)
				eid.MustCreateIfNotExists(ctx)
				n := mockBuildNotification(eid)
				So(processNotificationsBatch(ctx, tjNotifier, []notification{n}), ShouldBeNil)
				So(n.(*mockMessage).ackCount, ShouldEqual, 1)
				So(n.(*mockMessage).nackCount, ShouldEqual, 0)
				So(tjNotifier.ok, ShouldHaveLength, 1)
				So(tjNotifier.callCount, ShouldEqual, 1)
			})
			Convey("Irrelevant", func() {
				eid := tryjob.MustBuildbucketID(successHost, 404)
				n := mockBuildNotification(eid)
				So(processNotificationsBatch(ctx, tjNotifier, []notification{n}), ShouldBeNil)
				So(n.(*mockMessage).ackCount, ShouldEqual, 1)
				So(n.(*mockMessage).nackCount, ShouldEqual, 0)
				So(tjNotifier.callCount, ShouldEqual, 0)
			})
			Convey("Mixed batch", func() {
				batchSize := int64(100)
				startBuild := int64(1000)
				notifications := make([]notification, 0, batchSize)
				expectedSuccessIds := make(stringset.Set)
				expectedPermFailIds := make(stringset.Set)
				expectedTransFailIds := make(stringset.Set)
				for i := startBuild; i < startBuild+batchSize; i++ {
					eid := tryjob.MustBuildbucketID(successHost, i)
					switch i % 4 {
					case 0:
						eid.MustCreateIfNotExists(ctx)
						expectedSuccessIds.Add(string(eid))
					case 1:
						eid = tryjob.MustBuildbucketID(transFailHost, i)
						eid.MustCreateIfNotExists(ctx)
						expectedTransFailIds.Add(string(eid))
					case 3:
						eid = tryjob.MustBuildbucketID(permFailHost, i)
						eid.MustCreateIfNotExists(ctx)
						expectedPermFailIds.Add(string(eid))
					default:
					}
					notifications = append(notifications, mockBuildNotification(eid))
				}
				So(processNotificationsBatch(ctx, tjNotifier, notifications), ShouldErrLike, "permanent error")

				So(tjNotifier.ok.ToSortedSlice(), ShouldResemble, expectedSuccessIds.ToSortedSlice())
				So(tjNotifier.trans.ToSortedSlice(), ShouldResemble, expectedTransFailIds.ToSortedSlice())
				So(tjNotifier.perm.ToSortedSlice(), ShouldResemble, expectedPermFailIds.ToSortedSlice())

				var allAcks, allNacks int
				for _, n := range notifications {
					allAcks += n.(*mockMessage).ackCount
					allNacks += n.(*mockMessage).nackCount
				}
				So(allNacks, ShouldEqual, len(expectedTransFailIds))
				So(allAcks, ShouldEqual, len(notifications)-len(expectedTransFailIds))
			})
		})
		Convey("Transient failure", func() {
			Convey("Schedule call fails transiently", func() {
				eid := tryjob.MustBuildbucketID(transFailHost, 1)
				eid.MustCreateIfNotExists(ctx)
				n := mockBuildNotification(eid)
				So(processNotificationsBatch(ctx, tjNotifier, []notification{n}), ShouldBeNil)
				So(n.(*mockMessage).ackCount, ShouldEqual, 0)
				So(n.(*mockMessage).nackCount, ShouldEqual, 1)
				So(tjNotifier.trans, ShouldHaveLength, 1)
				So(tjNotifier.callCount, ShouldEqual, 1)
			})
		})
		Convey("Permanent failure", func() {
			Convey("Unparseable", func() {
				var n notification = &mockMessage{data: []byte("Unparseable hot garbage.'}]\"")}
				So(processNotificationsBatch(ctx, tjNotifier, []notification{n}), ShouldErrLike, "while unmarshalling build notification")
				So(n.(*mockMessage).ackCount, ShouldEqual, 1)
				So(n.(*mockMessage).nackCount, ShouldEqual, 0)
				So(tjNotifier.callCount, ShouldEqual, 0)
			})

			Convey("Schedule call fails permanently", func() {
				eid := tryjob.MustBuildbucketID(permFailHost, 1)
				eid.MustCreateIfNotExists(ctx)
				n := mockBuildNotification(eid)
				So(processNotificationsBatch(ctx, tjNotifier, []notification{n}), ShouldEqual, errPermanent)
				So(n.(*mockMessage).ackCount, ShouldEqual, 1)
				So(n.(*mockMessage).nackCount, ShouldEqual, 0)
				So(tjNotifier.perm, ShouldHaveLength, 1)
				So(tjNotifier.callCount, ShouldEqual, 1)
			})
		})

	})
}

func mockBuildNotification(eid tryjob.ExternalID) notification {
	host, id, err := eid.ParseBuildbucketID()
	if err != nil {
		panic(err)
	}
	buildJSON, err := json.Marshal(buildMessage{Build: &v1.LegacyApiCommonBuildMessage{Id: id}, Hostname: host})
	if err != nil {
		panic(err)
	}
	return &mockMessage{data: buildJSON}
}

type testTryjobNotifier struct {
	sync.Mutex
	trans, perm, ok stringset.Set
	callCount       int
}

// Schedule mocks tryjob.Schedule, and returns an error based on the host in
// the given ExternalID.
func (ttn *testTryjobNotifier) ScheduleUpdate(ctx context.Context, _ common.TryjobID, eid tryjob.ExternalID) error {
	ttn.Lock()
	defer ttn.Unlock()
	ttn.callCount++
	switch host, _, err := eid.ParseBuildbucketID(); {
	case err != nil:
		panic(err)
	case host == transFailHost:
		ttn.trans.Add(string(eid))
		return errTransient
	case host == permFailHost:
		ttn.perm.Add(string(eid))
		return errPermanent
	default:
		ttn.ok.Add(string(eid))
		return nil
	}
}

var (
	errTransient error = transient.Tag.Apply(errors.New("transient error"))
	errPermanent error = errors.New("permanent error")
)

type mockMessage struct {
	data      []byte
	ackCount  int
	nackCount int
}

func (mm *mockMessage) Ack() {
	mm.ackCount = 1
}
func (mm *mockMessage) Nack() {
	mm.nackCount = 1
}
func (mm *mockMessage) GetData() []byte {
	return mm.data
}

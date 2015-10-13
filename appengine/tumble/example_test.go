// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/bit_field"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type User struct {
	Name string `gae:"$id"`
}

func (u *User) SendMessage(c context.Context, msg string, toUsers ...string) (*OutgoingMessage, error) {
	sort.Strings(toUsers)
	ds := datastore.Get(c)
	k := ds.KeyForObj(u)
	outMsg := &OutgoingMessage{
		FromUser:   k,
		Message:    msg,
		Recipients: toUsers,
		Success:    bf.Make(uint64(len(toUsers))),
		Failure:    bf.Make(uint64(len(toUsers))),
	}
	err := EnterTransaction(c, k, func(c context.Context) ([]Mutation, error) {
		ds := datastore.Get(c)
		if err := ds.Put(outMsg); err != nil {
			return nil, err
		}
		outKey := ds.KeyForObj(outMsg)
		muts := make([]Mutation, len(toUsers))
		for i := range muts {
			muts[i] = &SendMessage{outKey, toUsers[i]}
		}
		return muts, nil
	})
	if err != nil {
		outMsg = nil
	}
	return outMsg, err
}

type OutgoingMessage struct {
	// datastore-assigned
	ID       int64          `gae:"$id"`
	FromUser *datastore.Key `gae:"$parent"`

	Message    string   `gae:",noindex"`
	Recipients []string `gae:",noindex"`

	Success bf.BitField
	Failure bf.BitField
}

type IncomingMessage struct {
	// OtherUser|OutgoingMessageID
	ID      string         `gae:"$id"`
	ForUser *datastore.Key `gae:"$parent"`
}

type SendMessage struct {
	Message *datastore.Key
	ToUser  string
}

func (m *SendMessage) Root(ctx context.Context) *datastore.Key {
	return datastore.Get(ctx).KeyForObj(&User{Name: m.ToUser})
}

func (m *SendMessage) RollForward(c context.Context) ([]Mutation, error) {
	ds := datastore.Get(c)
	u := &User{Name: m.ToUser}
	if err := ds.Get(u); err != nil {
		if err == datastore.ErrNoSuchEntity {
			return []Mutation{&WriteReceipt{m.Message, m.ToUser, false}}, nil
		}
		return nil, err
	}
	im := &IncomingMessage{
		ID:      fmt.Sprintf("%s|%d", m.Message.Parent().StringID(), m.Message.IntID()),
		ForUser: ds.KeyForObj(&User{Name: m.ToUser}),
	}
	err := ds.Get(im)
	if err == datastore.ErrNoSuchEntity {
		err = ds.Put(im)
		return []Mutation{&WriteReceipt{m.Message, m.ToUser, true}}, err
	}
	return nil, err
}

type WriteReceipt struct {
	Message   *datastore.Key
	Recipient string
	Success   bool
}

func (w *WriteReceipt) Root(ctx context.Context) *datastore.Key {
	return w.Message.Root()
}

func (w *WriteReceipt) RollForward(c context.Context) ([]Mutation, error) {
	ds := datastore.Get(c)
	m := &OutgoingMessage{ID: w.Message.IntID(), FromUser: w.Message.Parent()}
	if err := ds.Get(m); err != nil {
		return nil, err
	}

	idx := uint64(sort.SearchStrings(m.Recipients, w.Recipient))
	if w.Success {
		m.Success.Set(idx)
	} else {
		m.Failure.Set(idx)
	}

	return nil, ds.Put(m)
}

func init() {
	Register((*SendMessage)(nil))
	Register((*WriteReceipt)(nil))

	dustSettleTimeout = 0
}

func TestHighLevel(t *testing.T) {
	t.Parallel()

	Convey("Tumble", t, func() {
		Convey("Check registration", func() {
			So(registry, ShouldContainKey, "*tumble.SendMessage")
		})

		Convey("Good", func() {
			ctx := memory.Use(memlogger.Use(context.Background()))
			ctx, clk := testclock.UseTime(ctx, testclock.TestTimeUTC)
			cfg := GetConfig(ctx)
			ds := datastore.Get(ctx)
			tq := taskqueue.Get(ctx)
			l := logging.Get(ctx).(*memlogger.MemLogger)
			_ = l

			tq.Testable().CreateQueue(cfg.Name)

			ds.Testable().AddIndexes(&datastore.IndexDefinition{
				Kind: "tumble.Mutation",
				SortBy: []datastore.IndexColumn{
					{Property: "ExpandedShard"},
					{Property: "TargetRoot"},
				},
			})
			ds.Testable().CatchupIndexes()

			iterate := func() int {
				ret := 0
				tsks := tq.Testable().GetScheduledTasks()[cfg.Name]
				for _, tsk := range tsks {
					if tsk.ETA.After(clk.Now()) {
						continue
					}
					toks := strings.Split(tsk.Path, "/")
					rec := httptest.NewRecorder()
					ProcessShardHandler(ctx, rec, &http.Request{
						Header: http.Header{"X-AppEngine-QueueName": []string{cfg.Name}},
					}, httprouter.Params{
						{Key: "shard_id", Value: toks[4]},
						{Key: "timestamp", Value: toks[6]},
					})
					So(rec.Code, ShouldEqual, 200)
					So(tq.Delete(tsk, cfg.Name), ShouldBeNil)
					ret++
				}
				return ret
			}

			cron := func() {
				rec := httptest.NewRecorder()
				FireAllTasksHandler(ctx, rec, &http.Request{
					Header: http.Header{"X-Appengine-Cron": []string{"true"}},
				}, nil)
				So(rec.Code, ShouldEqual, 200)
			}

			charlie := &User{Name: "charlie"}
			So(ds.Put(charlie), ShouldBeNil)

			Convey("can't send to someone who doesn't exist", func() {
				outMsg, err := charlie.SendMessage(ctx, "Hey there", "lennon")
				So(err, ShouldBeNil)

				// need to advance clock and catch up indexes
				So(iterate(), ShouldEqual, 0)
				clk.Add(time.Second * 10)

				// need to catch up indexes
				So(iterate(), ShouldEqual, 1)

				cron()
				ds.Testable().CatchupIndexes()
				clk.Add(time.Second * 10)

				So(iterate(), ShouldEqual, cfg.NumShards)
				ds.Testable().CatchupIndexes()
				clk.Add(time.Second * 10)

				So(iterate(), ShouldEqual, 1)

				So(ds.Get(outMsg), ShouldBeNil)
				So(outMsg.Failure.All(true), ShouldBeTrue)
			})

			Convey("sending to yourself could be done in one iteration if you're lucky", func() {
				ds.Testable().Consistent(true)

				outMsg, err := charlie.SendMessage(ctx, "Hey there", "charlie")
				So(err, ShouldBeNil)

				clk.Add(time.Second * 10)

				So(iterate(), ShouldEqual, 1)

				So(ds.Get(outMsg), ShouldBeNil)
				So(outMsg.Success.All(true), ShouldBeTrue)
			})

			Convey("different version IDs log a warning", func() {
				ds.Testable().Consistent(true)

				outMsg, err := charlie.SendMessage(ctx, "Hey there", "charlie")
				So(err, ShouldBeNil)

				rm := &realMutation{
					ID:     "0000000000000001_00000000",
					Parent: ds.KeyForObj(charlie),
				}
				So(ds.Get(rm), ShouldBeNil)
				So(rm.Version, ShouldEqual, "testVersionID.1")
				rm.Version = "otherCodeVersion.1"
				So(ds.Put(rm), ShouldBeNil)

				clk.Add(time.Second * 10)

				l.Reset()
				So(iterate(), ShouldEqual, 1)
				So(l.Has(logging.Warning, "loading mutation with different code version", map[string]interface{}{
					"key":         "tumble.23.lock",
					"clientID":    "-62132730888_23",
					"mut_version": "otherCodeVersion.1",
					"cur_version": "testVersionID.1",
				}), ShouldBeTrue)

				So(ds.Get(outMsg), ShouldBeNil)
				So(outMsg.Success.All(true), ShouldBeTrue)
			})

			Convey("sending to 200 people is no big deal", func() {
				users := make([]User, 200)
				recipients := make([]string, 200)
				for i := range recipients {
					name := base64.StdEncoding.EncodeToString([]byte{byte(i)})
					recipients[i] = name
					users[i].Name = name
				}
				So(ds.PutMulti(users), ShouldBeNil)

				outMsg, err := charlie.SendMessage(ctx, "Hey there", recipients...)
				So(err, ShouldBeNil)

				// do all the SendMessages
				ds.Testable().CatchupIndexes()
				clk.Add(time.Second * 10)
				So(iterate(), ShouldEqual, cfg.NumShards)

				// do all the WriteReceipts
				l.Reset()
				ds.Testable().CatchupIndexes()
				clk.Add(time.Second * 10)
				So(iterate(), ShouldEqual, 1)

				// hacky proof that all 200 incoming message reciepts were buffered
				// appropriately.
				So(l.Has(logging.Info, "successfully processed 128 mutations, adding 0 more", map[string]interface{}{
					"key":      "tumble.23.lock",
					"clientID": "-62132730880_23",
				}), ShouldBeTrue)
				So(l.Has(logging.Info, "successfully processed 72 mutations, adding 0 more", map[string]interface{}{
					"key":      "tumble.23.lock",
					"clientID": "-62132730880_23",
				}), ShouldBeTrue)

				So(ds.Get(outMsg), ShouldBeNil)
				So(outMsg.Success.All(true), ShouldBeTrue)
				So(outMsg.Success.Size(), ShouldEqual, 200)

			})

		})

	})
}

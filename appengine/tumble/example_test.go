// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/bit_field"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type User struct {
	Name string `gae:"$id"`
}

type WriteMessage struct {
	Out *OutgoingMessage
}

func (w *WriteMessage) Root(context.Context) *datastore.Key {
	return w.Out.FromUser
}

func (w *WriteMessage) RollForward(c context.Context) ([]Mutation, error) {
	ds := datastore.Get(c)
	if err := ds.Put(w.Out); err != nil {
		return nil, err
	}
	outKey := ds.KeyForObj(w.Out)
	muts := make([]Mutation, len(w.Out.Recipients))
	for i, p := range w.Out.Recipients {
		muts[i] = &SendMessage{outKey, p}
	}
	return muts, nil
}

func (u *User) MakeOutgoingMessage(c context.Context, msg string, toUsers ...string) *OutgoingMessage {
	sort.Strings(toUsers)

	return &OutgoingMessage{
		FromUser:   datastore.Get(c).KeyForObj(u),
		Message:    msg,
		Recipients: toUsers,
		Success:    bf.Make(uint32(len(toUsers))),
		Failure:    bf.Make(uint32(len(toUsers))),
	}
}

func (u *User) SendMessage(c context.Context, msg string, toUsers ...string) (*OutgoingMessage, error) {
	outMsg := u.MakeOutgoingMessage(c, msg, toUsers...)

	err := RunMutation(c, &WriteMessage{outMsg})
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

	Notified bool
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

	idx := uint32(sort.SearchStrings(m.Recipients, w.Recipient))
	if w.Success {
		m.Success.Set(idx)
	} else {
		m.Failure.Set(idx)
	}

	err := ds.Put(m)
	if err != nil {
		return nil, err
	}

	if m.Success.CountSet()+m.Failure.CountSet() == uint32(len(m.Recipients)) {
		return []Mutation{&ReminderMessage{
			w.Message, m.FromUser.StringID(), clock.Now(c).UTC().Add(time.Minute * 5)},
		}, nil
	}

	return nil, nil
}

type ReminderMessage struct {
	Message   *datastore.Key
	Recipient string
	When      time.Time
}

var _ DelayedMutation = (*ReminderMessage)(nil)

func (r *ReminderMessage) Root(ctx context.Context) *datastore.Key {
	return r.Message.Root()
}

func (r *ReminderMessage) RollForward(c context.Context) ([]Mutation, error) {
	ds := datastore.Get(c)
	m := &OutgoingMessage{ID: r.Message.IntID(), FromUser: r.Message.Parent()}
	if err := ds.Get(m); err != nil {
		return nil, err
	}
	if m.Notified {
		return nil, nil
	}
	m.Notified = true
	return nil, ds.Put(m)
}

func (r *ReminderMessage) HighPriority() bool      { return false }
func (r *ReminderMessage) ProcessAfter() time.Time { return r.When }

// Embedder is just to prove that gob doesn't flip out when serializing
// Mutations within Mutations now. Presumably in a real instance of this you
// would have some other fields and do some general bookkeeping in Root() before
// returning Next from RollForward.
type Embedder struct {
	Next Mutation
}

func (*Embedder) Root(c context.Context) *datastore.Key {
	return datastore.Get(c).MakeKey("GeneralBookkeeping", 1)
}

func (e *Embedder) RollForward(context.Context) ([]Mutation, error) {
	// do something inside of Root()
	return []Mutation{e.Next}, nil
}

// NotRegistered is just to prove that gob does flip out if we don't
// gob.Register Mutations
type NotRegistered struct{}

func (*NotRegistered) Root(c context.Context) *datastore.Key           { return nil }
func (*NotRegistered) RollForward(context.Context) ([]Mutation, error) { return nil, nil }

func init() {
	Register((*WriteMessage)(nil))
	Register((*SendMessage)(nil))
	Register((*ReminderMessage)(nil))
	Register((*WriteReceipt)(nil))
	Register((*Embedder)(nil))
}

func testHighLevelImpl(t *testing.T, namespaces []string) {
	Convey("Tumble", t, func() {

		Convey("Check registration", func() {
			So(registry, ShouldContainKey, "*tumble.SendMessage")

			Convey("registered mutations can be embedded within each other", func() {
				buf := &bytes.Buffer{}
				enc := gob.NewEncoder(buf)
				So(enc.Encode(&Embedder{&WriteMessage{}}), ShouldBeNil)
				So(enc.Encode(&Embedder{&NotRegistered{}}), ShouldErrLike,
					"type not registered for interface")
			})
		})

		Convey("Good", func() {
			testing := NewTesting()
			ctx := testing.Context()

			if namespaces == nil {
				// Non-namespaced test.
				namespaces = []string{""}
			} else {
				testing.Config.Namespaced = true
			}

			testing.Service.Namespaces = func(context.Context) ([]string, error) {
				return namespaces, nil
			}

			forEachNS := func(c context.Context, f func(context.Context, int)) {
				for i, ns := range namespaces {
					nc := c
					if ns != "" {
						nc = info.Get(c).MustNamespace(ns)
					}
					f(nc, i)
				}
			}

			outMsgs := make([]*OutgoingMessage, len(namespaces))

			l := logging.Get(ctx).(*memlogger.MemLogger)
			_ = l

			charlie := &User{Name: "charlie"}
			forEachNS(ctx, func(ctx context.Context, i int) {
				So(datastore.Get(ctx).Put(charlie), ShouldBeNil)
			})

			// gds is a default-namespace datastore instance for global operations.
			gds := datastore.Get(ctx)

			Convey("can't send to someone who doesn't exist", func() {
				gds.Testable().Consistent(false)

				forEachNS(ctx, func(ctx context.Context, i int) {
					var err error
					outMsgs[i], err = charlie.SendMessage(ctx, "Hey there", "lennon")
					So(err, ShouldBeNil)
				})

				// need to advance clock and catch up indexes
				So(testing.Iterate(ctx), ShouldEqual, 0)
				testing.AdvanceTime(ctx)

				// need to catch up indexes
				So(testing.Iterate(ctx), ShouldBeGreaterThan, 0)

				testing.FireAllTasks(ctx)
				gds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)

				So(testing.Iterate(ctx), ShouldEqual, testing.NumShards)
				gds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)

				So(testing.Iterate(ctx), ShouldBeGreaterThan, 0)

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(datastore.Get(ctx).Get(outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Failure.All(true), ShouldBeTrue)
				})
			})

			Convey("sending to yourself could be done in one iteration", func() {
				forEachNS(ctx, func(ctx context.Context, i int) {
					var err error
					outMsgs[i], err = charlie.SendMessage(ctx, "Hey there", "charlie")
					So(err, ShouldBeNil)
				})

				testing.AdvanceTime(ctx)

				So(testing.Iterate(ctx), ShouldBeGreaterThan, 0)

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(datastore.Get(ctx).Get(outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Success.All(true), ShouldBeTrue)
				})
			})

			Convey("different version IDs log a warning", func() {
				forEachNS(ctx, func(ctx context.Context, i int) {
					var err error
					outMsgs[i], err = charlie.SendMessage(ctx, "Hey there", "charlie")
					So(err, ShouldBeNil)

					ds := datastore.Get(ctx)
					rm := &realMutation{
						ID:     "0000000000000001_00000000_00000000",
						Parent: ds.KeyForObj(charlie),
					}
					So(ds.Get(rm), ShouldBeNil)
					So(rm.Version, ShouldEqual, "testVersionID")
					rm.Version = "otherCodeVersion"
					So(ds.Put(rm), ShouldBeNil)
				})

				l.Reset()
				testing.Drain(ctx)

				if !testing.Config.Namespaced {
					So(l.Has(logging.Warning, "loading mutation with different code version", map[string]interface{}{
						"key":         "tumble.23.lock",
						"clientID":    "-62132730888_23",
						"mut_version": "otherCodeVersion",
						"cur_version": "testVersionID",
					}), ShouldBeTrue)
				} else {
					// TODO: Assert log messages for namespaced test?
				}

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(datastore.Get(ctx).Get(outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Success.All(true), ShouldBeTrue)
				})
			})

			Convey("sending to 200 people is no big deal", func() {
				gds.Testable().Consistent(false)

				users := make([]User, 200)
				recipients := make([]string, 200)
				for i := range recipients {
					name := base64.StdEncoding.EncodeToString([]byte{byte(i)})
					recipients[i] = name
					users[i].Name = name
				}

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(datastore.Get(ctx).PutMulti(users), ShouldBeNil)

					var err error
					outMsgs[i], err = charlie.SendMessage(ctx, "Hey there", recipients...)
					So(err, ShouldBeNil)
				})

				// do all the SendMessages
				gds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)
				So(testing.Iterate(ctx), ShouldEqual, testing.NumShards)

				// do all the WriteReceipts
				l.Reset()
				gds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)
				So(testing.Iterate(ctx), ShouldBeGreaterThan, 0)

				// hacky proof that all 200 incoming message reciepts were buffered
				// appropriately.
				//
				// The extra mutation at the end is supposed to be delayed, but we
				// didn't enable Delayed messages in this config.
				if !testing.Config.Namespaced {
					So(l.Has(logging.Info, "successfully processed 128 mutations (0 tail-call), adding 0 more", map[string]interface{}{
						"key":      "tumble.23.lock",
						"clientID": "-62132730884_23",
					}), ShouldBeTrue)
					So(l.Has(logging.Info, "successfully processed 73 mutations (1 tail-call), adding 0 more", map[string]interface{}{
						"key":      "tumble.23.lock",
						"clientID": "-62132730884_23",
					}), ShouldBeTrue)
				} else {
					// TODO: Assert log messages for namespaced test?
				}

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(datastore.Get(ctx).Get(outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Success.All(true), ShouldBeTrue)
					So(outMsgs[i].Success.Size(), ShouldEqual, 200)
				})
			})

			Convey("delaying messages works", func() {
				gds.Testable().Consistent(false)
				testing.DelayedMutations = true

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(datastore.Get(ctx).PutMulti([]User{
						{"sender"},
						{"recipient"},
					}), ShouldBeNil)
				})

				forEachNS(ctx, func(ctx context.Context, i int) {
					var err error
					outMsgs[i], err = charlie.SendMessage(ctx, "Hey there", "recipient")
					So(err, ShouldBeNil)
				})

				// do all the SendMessages
				l.Reset()
				gds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)

				// forgot to add the extra index!
				So(func() { testing.Iterate(ctx) }, ShouldPanicLike, "Insufficient indexes")
				gds.Testable().AddIndexes(&datastore.IndexDefinition{
					Kind: "tumble.Mutation",
					SortBy: []datastore.IndexColumn{
						{Property: "TargetRoot"},
						{Property: "ProcessAfter"},
					},
				})
				gds.Testable().CatchupIndexes()

				So(testing.Iterate(ctx), ShouldBeGreaterThan, 0)

				// do all the WriteReceipts
				gds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)
				So(testing.Iterate(ctx), ShouldBeGreaterThan, 0)

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(datastore.Get(ctx).Get(outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Success.All(true), ShouldBeTrue)
					So(outMsgs[i].Success.Size(), ShouldEqual, 1)
					So(outMsgs[i].Notified, ShouldBeFalse)
				})

				// Running another iteration should find nothing
				l.Reset()
				gds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)
				So(testing.Iterate(ctx), ShouldEqual, 0)

				if !testing.Config.Namespaced {
					So(l.Has(
						logging.Info, "skipping task: "+processURL(-62132730576, 23), nil,
					), ShouldBeTrue)
				} else {
					// TODO: Assert log messages for namespaced test?
				}

				// Now it'll find something
				gds.Testable().CatchupIndexes()
				clock.Get(ctx).(testclock.TestClock).Add(time.Minute * 5)
				So(testing.Iterate(ctx), ShouldBeGreaterThan, 0)

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(datastore.Get(ctx).Get(outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Notified, ShouldBeTrue)
				})
			})

		})

	})
}

func TestHighLevel(t *testing.T) {
	t.Parallel()

	testHighLevelImpl(t, nil)
}

func TestHighLevelMultiNamespace(t *testing.T) {
	t.Parallel()

	testHighLevelImpl(t, []string{"foo", "bar"})
}

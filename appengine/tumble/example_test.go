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

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/bit_field"
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

func (u *User) SendMessage(c context.Context, msg string, toUsers ...string) (*OutgoingMessage, error) {
	sort.Strings(toUsers)

	outMsg := &OutgoingMessage{
		FromUser:   datastore.Get(c).KeyForObj(u),
		Message:    msg,
		Recipients: toUsers,
		Success:    bf.Make(uint64(len(toUsers))),
		Failure:    bf.Make(uint64(len(toUsers))),
	}

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
	Register((*WriteReceipt)(nil))
	Register((*Embedder)(nil))
}

func TestHighLevel(t *testing.T) {
	t.Parallel()

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
			testing := Testing{}

			ctx := testing.Context()

			cfg := GetConfig(ctx)
			ds := datastore.Get(ctx)
			l := logging.Get(ctx).(*memlogger.MemLogger)
			_ = l

			charlie := &User{Name: "charlie"}
			So(ds.Put(charlie), ShouldBeNil)

			Convey("can't send to someone who doesn't exist", func() {
				ds.Testable().Consistent(false)

				outMsg, err := charlie.SendMessage(ctx, "Hey there", "lennon")
				So(err, ShouldBeNil)

				// need to advance clock and catch up indexes
				So(testing.Iterate(ctx), ShouldEqual, 0)
				testing.AdvanceTime(ctx)

				// need to catch up indexes
				So(testing.Iterate(ctx), ShouldEqual, 1)

				testing.FireAllTasks(ctx)
				ds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)

				So(testing.Iterate(ctx), ShouldEqual, cfg.NumShards)
				ds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)

				So(testing.Iterate(ctx), ShouldEqual, 1)

				So(ds.Get(outMsg), ShouldBeNil)
				So(outMsg.Failure.All(true), ShouldBeTrue)
			})

			Convey("sending to yourself could be done in one iteration", func() {
				outMsg, err := charlie.SendMessage(ctx, "Hey there", "charlie")
				So(err, ShouldBeNil)

				testing.AdvanceTime(ctx)

				So(testing.Iterate(ctx), ShouldEqual, 1)

				So(ds.Get(outMsg), ShouldBeNil)
				So(outMsg.Success.All(true), ShouldBeTrue)
			})

			Convey("different version IDs log a warning", func() {
				outMsg, err := charlie.SendMessage(ctx, "Hey there", "charlie")
				So(err, ShouldBeNil)

				rm := &realMutation{
					ID:     "0000000000000001_00000000_00000000",
					Parent: ds.KeyForObj(charlie),
				}
				So(ds.Get(rm), ShouldBeNil)
				So(rm.Version, ShouldEqual, "testVersionID.1")
				rm.Version = "otherCodeVersion.1"
				So(ds.Put(rm), ShouldBeNil)

				l.Reset()
				testing.Drain(ctx)
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
				ds.Testable().Consistent(false)

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
				testing.AdvanceTime(ctx)
				So(testing.Iterate(ctx), ShouldEqual, cfg.NumShards)

				// do all the WriteReceipts
				l.Reset()
				ds.Testable().CatchupIndexes()
				testing.AdvanceTime(ctx)
				So(testing.Iterate(ctx), ShouldEqual, 1)

				// hacky proof that all 200 incoming message reciepts were buffered
				// appropriately.
				So(l.Has(logging.Info, "successfully processed 128 mutations (0 tail-call), adding 0 more", map[string]interface{}{
					"key":      "tumble.23.lock",
					"clientID": "-62132730884_23",
				}), ShouldBeTrue)
				So(l.Has(logging.Info, "successfully processed 72 mutations (0 tail-call), adding 0 more", map[string]interface{}{
					"key":      "tumble.23.lock",
					"clientID": "-62132730884_23",
				}), ShouldBeTrue)

				So(ds.Get(outMsg), ShouldBeNil)
				So(outMsg.Success.All(true), ShouldBeTrue)
				So(outMsg.Success.Size(), ShouldEqual, 200)

			})

		})

	})
}

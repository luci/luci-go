// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tumble

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/data/bit_field"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

type User struct {
	Name string `gae:"$id"`
}

type TimeoutMessageSend struct {
	Out       *ds.Key
	WaitUntil time.Time
}

func (t *TimeoutMessageSend) ProcessAfter() time.Time { return t.WaitUntil }
func (t *TimeoutMessageSend) HighPriority() bool      { return false }

func (t *TimeoutMessageSend) Root(context.Context) *ds.Key {
	return t.Out.Root()
}

func (t *TimeoutMessageSend) RollForward(c context.Context) ([]Mutation, error) {
	logging.Warningf(c, "TimeoutMessageSend.RollForward(%s)", t.Out)
	out := &OutgoingMessage{}
	ds.PopulateKey(out, t.Out)
	if err := ds.Get(c, out); err != nil {
		return nil, err
	}
	if out.Notified || out.TimedOut {
		return nil, nil
	}
	out.TimedOut = true
	return nil, ds.Put(c, out)
}

type WriteMessage struct {
	Out *OutgoingMessage
}

func (w *WriteMessage) Root(context.Context) *ds.Key {
	return w.Out.FromUser
}

func (w *WriteMessage) RollForward(c context.Context) ([]Mutation, error) {
	if err := ds.Put(c, w.Out); err != nil {
		return nil, err
	}
	outKey := ds.KeyForObj(c, w.Out)
	muts := make([]Mutation, len(w.Out.Recipients))
	for i, p := range w.Out.Recipients {
		muts[i] = &SendMessage{outKey, p, time.Time{}}
		if p == "slowmojoe" {
			muts[i].(*SendMessage).WaitUntil = clock.Now(c).Add(10 * time.Minute)
		}
	}
	return muts, PutNamedMutations(c, outKey, map[string]Mutation{
		"timeout": &TimeoutMessageSend{outKey, clock.Now(c).Add(5 * time.Minute)},
	})
}

func (u *User) MakeOutgoingMessage(c context.Context, msg string, toUsers ...string) *OutgoingMessage {
	sort.Strings(toUsers)

	return &OutgoingMessage{
		FromUser:   ds.KeyForObj(c, u),
		Message:    msg,
		Recipients: toUsers,
		Success:    bit_field.Make(uint32(len(toUsers))),
		Failure:    bit_field.Make(uint32(len(toUsers))),
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
	ID       int64   `gae:"$id"`
	FromUser *ds.Key `gae:"$parent"`

	Message    string   `gae:",noindex"`
	Recipients []string `gae:",noindex"`

	Success bit_field.BitField
	Failure bit_field.BitField

	Notified bool
	TimedOut bool
}

type IncomingMessage struct {
	// OtherUser|OutgoingMessageID
	ID      string  `gae:"$id"`
	ForUser *ds.Key `gae:"$parent"`
}

type SendMessage struct {
	Message   *ds.Key
	ToUser    string
	WaitUntil time.Time
}

func (m *SendMessage) Root(ctx context.Context) *ds.Key {
	return ds.KeyForObj(ctx, &User{Name: m.ToUser})
}

func (m *SendMessage) ProcessAfter() time.Time { return m.WaitUntil }
func (m *SendMessage) HighPriority() bool      { return false }

func (m *SendMessage) RollForward(c context.Context) ([]Mutation, error) {
	u := &User{Name: m.ToUser}
	if err := ds.Get(c, u); err != nil {
		if err == ds.ErrNoSuchEntity {
			return []Mutation{&WriteReceipt{m.Message, m.ToUser, false}}, nil
		}
		return nil, err
	}
	im := &IncomingMessage{
		ID:      fmt.Sprintf("%s|%d", m.Message.Parent().StringID(), m.Message.IntID()),
		ForUser: ds.KeyForObj(c, &User{Name: m.ToUser}),
	}
	err := ds.Get(c, im)
	if err == ds.ErrNoSuchEntity {
		err = ds.Put(c, im)
		return []Mutation{&WriteReceipt{m.Message, m.ToUser, true}}, err
	}
	return nil, err
}

type WriteReceipt struct {
	Message   *ds.Key
	Recipient string
	Success   bool
}

func (w *WriteReceipt) Root(ctx context.Context) *ds.Key {
	return w.Message.Root()
}

func (w *WriteReceipt) RollForward(c context.Context) ([]Mutation, error) {
	m := &OutgoingMessage{ID: w.Message.IntID(), FromUser: w.Message.Parent()}
	if err := ds.Get(c, m); err != nil {
		return nil, err
	}

	idx := uint32(sort.SearchStrings(m.Recipients, w.Recipient))
	if w.Success {
		m.Success.Set(idx)
	} else {
		m.Failure.Set(idx)
	}

	err := ds.Put(c, m)
	if err != nil {
		return nil, err
	}

	if m.Success.CountSet()+m.Failure.CountSet() == uint32(len(m.Recipients)) {
		err := CancelNamedMutations(c, w.Message, "timeout")
		muts := []Mutation{&ReminderMessage{
			w.Message, m.FromUser.StringID(), clock.Now(c).UTC().Add(time.Minute * 5)},
		}
		return muts, err
	}

	return nil, nil
}

type ReminderMessage struct {
	Message   *ds.Key
	Recipient string
	When      time.Time
}

var _ DelayedMutation = (*ReminderMessage)(nil)
var _ DelayedMutation = (*TimeoutMessageSend)(nil)
var _ DelayedMutation = (*SendMessage)(nil)

func (r *ReminderMessage) Root(ctx context.Context) *ds.Key {
	return r.Message.Root()
}

func (r *ReminderMessage) RollForward(c context.Context) ([]Mutation, error) {
	m := &OutgoingMessage{}
	ds.PopulateKey(m, r.Message)
	if err := ds.Get(c, m); err != nil {
		return nil, err
	}
	if m.Notified {
		return nil, nil
	}
	m.Notified = true
	return nil, ds.Put(c, m)
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

func (*Embedder) Root(c context.Context) *ds.Key {
	return ds.MakeKey(c, "GeneralBookkeeping", 1)
}

func (e *Embedder) RollForward(context.Context) ([]Mutation, error) {
	// do something inside of Root()
	return []Mutation{e.Next}, nil
}

// NotRegistered is just to prove that gob does flip out if we don't
// gob.Register Mutations
type NotRegistered struct{}

func (*NotRegistered) Root(c context.Context) *ds.Key                  { return nil }
func (*NotRegistered) RollForward(context.Context) ([]Mutation, error) { return nil, nil }

func init() {
	Register((*Embedder)(nil))
	Register((*ReminderMessage)(nil))
	Register((*SendMessage)(nil))
	Register((*TimeoutMessageSend)(nil))
	Register((*WriteMessage)(nil))
	Register((*WriteReceipt)(nil))
}

func shouldHaveLogMessage(actual interface{}, expected ...interface{}) string {
	l := actual.(*memlogger.MemLogger)
	if len(expected) != 1 {
		panic("expected must contain a single string")
	}
	exp := expected[0].(string)

	msgs := l.Messages()
	msgText := make([]string, len(msgs))
	for i, msg := range msgs {
		msgText[i] = msg.Msg
	}
	return ShouldContainSubstring(strings.Join(msgText, "\n"), exp)
}

func testHighLevelImpl(t *testing.T, namespaces []string) {
	FocusConvey("Tumble", t, func() {

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

		FocusConvey("Good", func() {
			testing := &Testing{}
			testing.Service.Namespaces = func(context.Context) ([]string, error) { return namespaces, nil }

			ctx := testing.Context()

			forEachNS := func(c context.Context, fn func(context.Context, int)) {
				for i, ns := range namespaces {
					fn(info.MustNamespace(c, ns), i)
				}
			}

			outMsgs := make([]*OutgoingMessage, len(namespaces))

			l := logging.Get(ctx).(*memlogger.MemLogger)

			charlie := &User{Name: "charlie"}
			forEachNS(ctx, func(ctx context.Context, i int) {
				So(ds.Put(ctx, charlie), ShouldBeNil)
			})

			// gctx is a default-namespace Context for global operations.
			gctx := ctx

			Convey("can't send to someone who doesn't exist", func() {
				forEachNS(ctx, func(ctx context.Context, i int) {
					var err error
					outMsgs[i], err = charlie.SendMessage(ctx, "Hey there", "lennon")
					So(err, ShouldBeNil)
				})

				testing.DrainAll(ctx)

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(ds.Get(ctx, outMsgs[i]), ShouldBeNil)
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
				So(testing.IterateAll(ctx), ShouldBeGreaterThan, 0)

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(ds.Get(ctx, outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Success.All(true), ShouldBeTrue)
				})
			})

			Convey("different version IDs log a warning", func() {
				forEachNS(ctx, func(ctx context.Context, i int) {
					var err error
					outMsgs[i], err = charlie.SendMessage(ctx, "Hey there", "charlie")
					So(err, ShouldBeNil)

					rm := &realMutation{
						ID:     "0000000000000001_00000000_00000000",
						Parent: ds.KeyForObj(ctx, charlie),
					}
					So(ds.Get(ctx, rm), ShouldBeNil)
					So(rm.Version, ShouldEqual, "testVersionID")
					rm.Version = "otherCodeVersion"
					So(ds.Put(ctx, rm), ShouldBeNil)
				})

				l.Reset()
				testing.DrainAll(ctx)
				So(l, shouldHaveLogMessage, "loading mutation with different code version")

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(ds.Get(ctx, outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Success.All(true), ShouldBeTrue)
				})
			})

			FocusConvey("sending to 100 people is no big deal", func() {
				ds.GetTestable(gctx).Consistent(false)

				users := make([]User, 100)
				recipients := make([]string, len(users))
				for i := range recipients {
					name := base64.StdEncoding.EncodeToString([]byte{byte(i)})
					recipients[i] = name
					users[i].Name = name
				}

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(ds.Put(ctx, users), ShouldBeNil)

					var err error
					outMsgs[i], err = charlie.SendMessage(ctx, "Hey there", recipients...)
					So(err, ShouldBeNil)
				})

				// do all the SendMessages
				ds.GetTestable(gctx).CatchupIndexes()
				testing.AdvanceTime(ctx)
				testing.IterateAll(ctx)

				// do all the WriteReceipts
				l.Reset()
				ds.GetTestable(gctx).CatchupIndexes()
				testing.AdvanceTime(ctx)
				So(testing.IterateAll(ctx), ShouldBeGreaterThan, 0)

				// hacky proof that all 100 incoming message receipts were buffered
				// appropriately, +1 for the outgoing tail call, which will be processed
				// immediately since delayed mutations are not enabled.
				So(l, shouldHaveLogMessage, "successfully processed 101 mutations (1 tail-call), delta 0")

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(ds.Get(ctx, outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Success.All(true), ShouldBeTrue)
					So(outMsgs[i].Success.Size(), ShouldEqual, len(users))
				})
			})

			Convey("delaying messages works", func() {
				ds.GetTestable(gctx).Consistent(false)
				cfg := testing.GetConfig(ctx)
				cfg.DelayedMutations = true
				testing.UpdateSettings(ctx, cfg)

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(ds.Put(ctx,
						&User{"recipient"},
					), ShouldBeNil)
				})

				forEachNS(ctx, func(ctx context.Context, i int) {
					var err error
					outMsgs[i], err = charlie.SendMessage(ctx, "Hey there", "recipient")
					So(err, ShouldBeNil)
				})

				// do all the SendMessages
				l.Reset()
				ds.GetTestable(gctx).CatchupIndexes()
				testing.AdvanceTime(ctx)

				// forgot to add the extra index!
				So(func() { testing.IterateAll(ctx) }, ShouldPanic)
				So(l, shouldHaveLogMessage, "Insufficient indexes")

				ds.GetTestable(gctx).AddIndexes(&ds.IndexDefinition{
					Kind: "tumble.Mutation",
					SortBy: []ds.IndexColumn{
						{Property: "TargetRoot"},
						{Property: "ProcessAfter"},
					},
				})
				ds.GetTestable(gctx).CatchupIndexes()

				So(testing.IterateAll(ctx), ShouldBeGreaterThan, 0)

				// do all the WriteReceipts
				ds.GetTestable(gctx).CatchupIndexes()
				testing.AdvanceTime(ctx)
				So(testing.IterateAll(ctx), ShouldBeGreaterThan, 0)

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(ds.Get(ctx, outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Success.All(true), ShouldBeTrue)
					So(outMsgs[i].Success.Size(), ShouldEqual, 1)
					So(outMsgs[i].Notified, ShouldBeFalse)
				})

				// Running another iteration should find nothing
				l.Reset()
				ds.GetTestable(gctx).CatchupIndexes()
				testing.AdvanceTime(ctx)
				So(testing.IterateAll(ctx), ShouldEqual, 0)
				So(l, shouldHaveLogMessage, "skipping task: ETA(0001-02-03 04:10:24 +0000 UTC)")

				// Now it'll find something
				ds.GetTestable(gctx).CatchupIndexes()
				clock.Get(ctx).(testclock.TestClock).Add(time.Minute * 5)
				So(testing.IterateAll(ctx), ShouldBeGreaterThan, 0)

				forEachNS(ctx, func(ctx context.Context, i int) {
					So(ds.Get(ctx, outMsgs[i]), ShouldBeNil)
					So(outMsgs[i].Notified, ShouldBeTrue)
				})
			})

			Convey("named mutations work", func() {
				if len(namespaces) > 1 {
					// Disable this test for multi-namespace case.
					return
				}

				testing.EnableDelayedMutations(ctx)

				So(ds.Put(gctx,
					&User{"recipient"},
					&User{"slowmojoe"},
				), ShouldBeNil)

				outMsg, err := charlie.SendMessage(ctx, "Hey there", "recipient", "slowmojoe")
				So(err, ShouldBeNil)

				testing.AdvanceTime(ctx)
				So(testing.IterateAll(ctx), ShouldEqual, 1) // sent to "recipient"

				testing.AdvanceTime(ctx)
				testing.AdvanceTime(ctx)
				So(testing.IterateAll(ctx), ShouldEqual, 1) // receipt from "recipient"

				clock.Get(ctx).(testclock.TestClock).Add(time.Minute * 5)
				So(testing.IterateAll(ctx), ShouldEqual, 1) // timeout!
				So(l.HasFunc(func(ent *memlogger.LogEntry) bool {
					return strings.Contains(ent.Msg, "TimeoutMessageSend")
				}), ShouldBeTrue)

				So(ds.Get(gctx, outMsg), ShouldBeNil)
				So(outMsg.TimedOut, ShouldBeTrue)
				So(outMsg.Notified, ShouldBeFalse)
				So(outMsg.Success.CountSet(), ShouldEqual, 1)

				clock.Get(ctx).(testclock.TestClock).Add(time.Minute * 5)
				So(testing.IterateAll(ctx), ShouldEqual, 1) // WriteReceipt slowmojoe

				testing.AdvanceTime(ctx)
				So(testing.IterateAll(ctx), ShouldEqual, 1) // Notification submitted
				So(ds.Get(gctx, outMsg), ShouldBeNil)
				So(outMsg.Failure.CountSet(), ShouldEqual, 0)
				So(outMsg.Success.CountSet(), ShouldEqual, 2)

				testing.AdvanceTime(ctx)
				clock.Get(ctx).(testclock.TestClock).Add(time.Minute * 5)
				So(testing.IterateAll(ctx), ShouldEqual, 1) // ReminderMessage set
				So(ds.Get(gctx, outMsg), ShouldBeNil)
				So(outMsg.Notified, ShouldBeTrue)

			})

			Convey("can cancel named mutations", func() {
				if len(namespaces) > 1 {
					// Disable this test for multi-namespace case.
					return
				}

				testing.EnableDelayedMutations(ctx)

				So(ds.Put(gctx,
					&User{"recipient"},
					&User{"other"},
				), ShouldBeNil)

				outMsg, err := charlie.SendMessage(ctx, "Hey there", "recipient", "other")
				So(err, ShouldBeNil)

				testing.AdvanceTime(ctx)
				So(testing.IterateAll(ctx), ShouldEqual, 2) // sent to "recipient" and "other"

				testing.AdvanceTime(ctx)
				testing.AdvanceTime(ctx)
				So(testing.IterateAll(ctx), ShouldEqual, 1) // receipt from "recipient" and "other", Notification pending

				testing.AdvanceTime(ctx)
				clock.Get(ctx).(testclock.TestClock).Add(time.Minute * 5)
				So(testing.IterateAll(ctx), ShouldEqual, 2) // ReminderMessage set

				clock.Get(ctx).(testclock.TestClock).Add(time.Minute * 5)
				So(testing.IterateAll(ctx), ShouldEqual, 0) // Nothing else

				So(ds.Get(gctx, outMsg), ShouldBeNil)
				So(outMsg.TimedOut, ShouldBeFalse)
				So(outMsg.Notified, ShouldBeTrue)
				So(outMsg.Success.CountSet(), ShouldEqual, 2)

				So(l.HasFunc(func(ent *memlogger.LogEntry) bool {
					return strings.Contains(ent.Msg, "TimeoutMessageSend")
				}), ShouldBeFalse)
			})

		})

	})
}

func TestHighLevelSingleNamespace(t *testing.T) {
	t.Parallel()

	testHighLevelImpl(t, []string{""})
}

func TestHighLevelMultiNamespace(t *testing.T) {
	t.Parallel()

	testHighLevelImpl(t, []string{"foo", "bar.baz_qux"})
}

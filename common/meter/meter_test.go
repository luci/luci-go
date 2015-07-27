// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meter

import (
	"fmt"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

// goconvey assertion that a slice of int ([]int) has the expected Value
// parameters.
func shouldHaveWork(actual interface{}, expected ...interface{}) string {
	sentWork := actual.([]interface{})

	expectedWork := make([]int, len(expected))
	for idx, exp := range expected {
		switch t := exp.(type) {
		case int:
			expectedWork[idx] = t
		default:
			panic("Unsupported work value type.")
		}
	}

	if len(sentWork) != len(expectedWork) {
		return fmt.Sprintf("Work count (%d) doesn't match expected (%d)", len(sentWork), len(expectedWork))
	}

	for idx, exp := range expectedWork {
		testSentWork := sentWork[idx].(int)
		if testSentWork != exp {
			return fmt.Sprintf("Work #%d content does not match expected: %d != %d",
				idx, testSentWork, exp)
		}
	}
	return ""
}

// Test a Meter instance.
func TestMeter(t *testing.T) {
	t.Parallel()

	Convey(`In a test environment`, t, func() {
		ctx, tc := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))

		// Setup a channel to acknowledge work for test synchronization. We will use
		// the work callback to trigger this channel.
		workC := make(chan interface{})
		workAckC := make(chan interface{})

		config := Config{
			Callback: func(bundle []interface{}) {
				workC <- bundle
			},
			IngestCallback: func(work interface{}) bool {
				workAckC <- work
				return false
			},
		}

		Convey(`Will panic if no callback is supplied.`, func() {
			config.Callback = nil
			So(func() { New(ctx, config) }, ShouldPanic)
		})

		Convey(`Timer: Will buffer work until the timer is signalled.`, func() {
			config.Delay = 1 * time.Second // Doesn't matter, non-zero will cause timer to be used.
			m := New(ctx, config)
			defer m.Stop()

			// Send three messages. None of them should be forwarded to the underlying
			// Output.
			for _, v := range []int{0, 1, 2} {
				m.AddWait(v)
				<-workAckC
			}

			// Signal our timer.
			tc.Add(1 * time.Second)
			// All three messages should be sent to our underlying Output at the same
			// time.
			So(<-workC, shouldHaveWork, 0, 1, 2)
		})

		Convey(`Flush: Will buffer messages until a flush is triggered.`, func() {
			m := New(ctx, config) // Will never auto-flush.
			defer m.Stop()

			// Send two messages. Nothing should be forwarded.
			m.AddWait(0)
			<-workAckC
			m.AddWait(1)
			<-workAckC

			// Send a third Work unit. We should receive a bundle of {0, 1, 2}.
			m.AddWait(2)
			<-workAckC

			m.Flush()
			So(<-workC, shouldHaveWork, 0, 1, 2)
		})

		Convey(`Count: Will buffer messages until count is reached.`, func() {
			config.Count = 3
			m := New(ctx, config)
			defer m.Stop()

			// Send two messages. Nothing should be forwarded.
			m.AddWait(0)
			<-workAckC
			m.AddWait(1)
			<-workAckC

			// Send a third message. Our underlying Output should receive the set of
			// three.
			m.AddWait(2)
			<-workAckC
			So(<-workC, shouldHaveWork, 0, 1, 2)
		})

		Convey(`WorkCallback: Will buffer messages until flushed via callback.`, func() {
			count := 0
			config.IngestCallback = func(interface{}) bool {
				count++
				return count >= 3
			}

			m := New(ctx, config)
			defer m.Stop()

			m.AddWait(0)
			m.AddWait(1)
			m.AddWait(2)
			So(<-workC, shouldHaveWork, 0, 1, 2)
		})

		Convey(`Configured with multiple constraints`, func() {
			config.Delay = 1 * time.Second // Doesn't matter, non-zero will cause timer to be used.
			config.Count = 3
			m := New(ctx, config)
			defer m.Stop()

			// Fill our buckets up to near threshold without dumping messages.
			m.AddWait(0)
			<-workAckC
			m.AddWait(1)
			<-workAckC

			// Hit count thresholds and flush at the same time.
			m.AddWait(2)
			<-workAckC
			m.Flush()
			So(<-workC, shouldHaveWork, 0, 1, 2)

			// Fill our buckets up to near threshold again.
			m.AddWait(3)
			<-workAckC
			m.AddWait(4)
			<-workAckC

			// Hit time threshold.
			tc.Add(1 * time.Second)
			So(<-workC, shouldHaveWork, 3, 4)

			// Hit count threshold.
			m.AddWait(5)
			<-workAckC
			m.AddWait(6)
			<-workAckC
			m.AddWait(7)
			<-workAckC
			So(<-workC, shouldHaveWork, 5, 6, 7)
		})

		Convey(`When full, will return ErrFull if not blocking.`, func() {
			m := newImpl(ctx, &config)

			// Fill up the work channel (do not reap workC).
			id := 0
			for i := 0; i < config.getAddBufferSize(); i++ {
				So(m.Add(i), ShouldBeNil)
				id++
			}

			// Add another work unit.
			So(m.Add(id+1), ShouldEqual, ErrFull)
		})
	})
}

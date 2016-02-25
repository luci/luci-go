// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package subscriber

import (
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
)

const (
	// DefaultNoDataDelay is the default amount of time a worker will sleep if
	// there is no Pub/Sub data.
	DefaultNoDataDelay = 5 * time.Second
)

// ACK is a goroutine-safe interface that is capable of sending Pub/Sub ACKs.
type ACK interface {
	// Ack ACKs a single Pub/Sub message ID.
	//
	// Note that this method cannot fail. The ACK instance is responsible for
	// making a best effort to perform the acknowledgement and buffering/retrying
	// as it sees fit. Applications must understand that ACKs can fail and plan
	// their ingest pipeline accordingly.
	//
	// This functions primarily as a hand-off of responsibility from Subscriber
	// (intent to acknowledge) to ACK (responsibility to acknowledge).
	Ack(string)
}

// Handler is a handler function that manages an individual message. It returns
// true if the message should be consumed and false otherwise.
type Handler func(*pubsub.Message) bool

// Subscriber pulls messages from a Pub/Sub channel and processes them.
type Subscriber struct {
	// S is used to pull Pub/Sub messages.
	S Source
	// A is used to send Pub/Sub message ACKs.
	A ACK

	// PullWorkers is the maximum number of simultaneous worker goroutines that a
	// Subscriber can have pulling Pub/Sub at any given moment.
	//
	// If <= 0, one worker will be used.
	PullWorkers int

	// HandlerWorkers is the maximum number of message processing workers that
	// the Subscriber will run at any given time. One worker is dispatched per
	// Pub/Sub message received.
	//
	// If <= 0, the number of handler workers will be unbounded.
	HandlerWorkers int

	// NoDataDelay is the amount of time to wait in between retries if there is
	// either an error or no data polling Pub/Sub.
	//
	// If <= 0, DefaultNoDataDelay will be used.
	NoDataDelay time.Duration

	// noDataMu is used to throttle retries if the subscription has no available
	// data.
	noDataMu sync.Mutex
}

// Run executes until the supplied Context is canceled. Each recieved message
// is processed by a Handler.
func (s *Subscriber) Run(c context.Context, h Handler) {
	pullWorkers := s.PullWorkers
	if pullWorkers <= 0 {
		pullWorkers = 1
	}

	runner := parallel.Runner{
		Sustained: s.HandlerWorkers,
		Maximum:   s.HandlerWorkers,
	}
	defer runner.Close()

	parallel.WorkPool(pullWorkers, func(taskC chan<- func() error) {
		for {
			// Stop if our Context has been canceled.
			if err := c.Err(); err != nil {
				return
			}

			// Fetch and process another batch of messages.
			taskC <- func() error {
				switch msgs, err := s.S.Pull(c); err {
				case context.Canceled, context.DeadlineExceeded:
					break

				case nil:
					s.handleMessages(c, h, &runner, msgs)

				default:
					log.WithError(err).Errorf(c, "Failed to pull messages.")
					s.noDataSleep(c)
				}

				return nil
			}
		}
	})
}

func (s *Subscriber) handleMessages(c context.Context, h Handler, r *parallel.Runner, msgs []*pubsub.Message) {
	if len(msgs) == 0 {
		s.noDataSleep(c)
		return
	}

	// Handle all messages in parallel.
	parallel.Ignore(r.Run(func(taskC chan<- func() error) {
		for _, msg := range msgs {
			msg := msg

			// Handle an individual message. If the Handler returns true, ACK
			// it.
			taskC <- func() error {
				if h(msg) {
					s.A.Ack(msg.AckID)
				}
				return nil
			}
		}
	}))
}

// noDataSleep sleeps for the configured NoDataDelay. This sleep will terminate
// immediately if the supplied Context is canceled.
//
// This method is called when a pull goroutine receives either an error or a
// response with no messages from Pub/Sub. In order to smooth out retry spam
// while we either wait for more messages or wait for Pub/Sub to work again, all
// of the goroutines share a sleep mutex.
//
// This collapses their potentially-parallel sleep attempts into a serial chain
// of sleeps. This is done by having all sleep attempts share a lock. Any
// goroutine that wants to sleep will wait for the lock and hold it through its
// sleep. This is a simple method to obtain the desired effect of avoiding
// pointless burst Pub/Sub spam when the service has nothing useful to offer.
func (s *Subscriber) noDataSleep(c context.Context) {
	s.noDataMu.Lock()
	defer s.noDataMu.Unlock()

	d := s.NoDataDelay
	if d <= 0 {
		d = DefaultNoDataDelay
	}
	clock.Sleep(c, d)
}

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package subscriber implements the Subscriber, which orchestrates parallel
// Pub/Sub subscription pulls.
package subscriber

import (
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/gcloud/gcps"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	"google.golang.org/cloud/pubsub"
)

const (
	// DefaultWorkers is the number of subscription workers to use.
	DefaultWorkers = 20

	// noDataDelay is the amount of time a worker will sleep if there is no
	// Pub/Sub data.
	noDataDelay = 1 * time.Second
)

// PubSubPull is an interface for something that can return Pub/Sub messages on
// request.
//
// gcps.PubSub naturally implements this interface.
type PubSubPull interface {
	// Pull pulls messages from the subscription. It returns up the requested
	// number of messages.
	Pull(gcps.Subscription, int) ([]*pubsub.Message, error)
}

// Callback is invoked for each received Pub/Sub message.
type Callback func(*pubsub.Message)

// Subscriber is an interface to return Pub/Sub messages from Cloud Pub/Sub.
// It spawns several worker goroutines to poll a Pub/Sub interface and invoke
// a configured callback for each received message.
type Subscriber struct {
	// PubSub is the underlying Pub/Sub instance to pull from.
	PubSub PubSubPull

	// Subscription is the name of the subscription to poll.
	Subscription gcps.Subscription

	// BatchSize is the number of simultaneous messages to pull. If <= zero,
	// gcps.MaxSubscriptionPullSize is used.
	BatchSize int

	// Workers is the number of Pub/Sub polling workers to simultaneously run. If
	// <= zero, DefaultWorkers will be used.
	Workers int
}

// Run executes the Subscriber instance, spawning several workers and polling
// for messages. The supplied callback will be invoked for each polled message.
//
// Subscriber will run until the supplied Context is cancelled.
func (s *Subscriber) Run(ctx context.Context, cb Callback) {
	batchSize := s.BatchSize
	if batchSize <= 0 {
		batchSize = gcps.MaxSubscriptionPullSize
	}

	// Set our base logging fields.
	ctx = log.SetFields(ctx, log.Fields{
		"subscription": s.Subscription,
		"batchSize":    batchSize,
	})

	// Mutex to protect Pub/Sub spamming when there are no available messages.
	noDataMu := sync.Mutex{}

	workers := s.Workers
	if workers <= 0 {
		workers = DefaultWorkers
	}
	err := parallel.WorkPool(workers, func(taskC chan<- func() error) {
		// Dispatch poll tasks until our Context is cancelled.
		for active := true; active; {
			// Check if we're cancelled.
			select {
			case <-ctx.Done():
				log.WithError(ctx.Err()).Infof(ctx, "Context is finished.")
				return
			default:
				break
			}

			// Dispatch a poll task. Always return "nil", even if there is an error,
			// since error conditions are neither tracked nor fatal.
			taskC <- func() error {
				if err := s.pullMessages(ctx, batchSize, &noDataMu, cb); err != nil {
					log.WithError(err).Errorf(ctx, "Failed to pull messages.")
				}
				return nil
			}
		}
	})
	if err != nil {
		log.WithError(err).Errorf(ctx, "Failed to run Subscriber work pool.")
	}
}

func (s *Subscriber) pullMessages(ctx context.Context, batchSize int, noDataMu sync.Locker, cb Callback) error {
	// Pull a set of messages.
	var messages []*pubsub.Message
	err := retry.Retry(ctx, retry.TransientOnly(retry.Default), func() (ierr error) {
		messages, ierr = s.PubSub.Pull(s.Subscription, batchSize)
		return
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
		}.Warningf(ctx, "Transient error on Pull(). Retrying...")
	})

	if len(messages) > 0 {
		log.Fields{
			"messageCount": len(messages),
		}.Infof(ctx, "Pulled messages.")

		for _, msg := range messages {
			cb(msg)
		}
	}

	if err != nil || len(messages) == 0 {
		log.Fields{
			log.ErrorKey: err,
			"delay":      noDataDelay,
		}.Debugf(ctx, "Sleeping.")

		noDataMu.Lock()
		defer noDataMu.Unlock()
		cancellableSleep(ctx, noDataDelay)
	}

	return err
}

// cancellableSleep sleeps, returning either when the sleep duration has expired
// or the supplied context has been cancelled.
func cancellableSleep(ctx context.Context, delay time.Duration) {
	// Sleep for "delay", stopping early if our Context is cancelled.
	select {
	case <-clock.After(ctx, delay):
		break

	case <-ctx.Done():
		break
	}
}

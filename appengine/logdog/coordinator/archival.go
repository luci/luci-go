// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/proto/google"
	"golang.org/x/net/context"
)

// ErrArchiveTasked is returned by ArchivalParams' PublishTask if the supplied
// LogStream indicates that it has already had an archival request dispatched.
var ErrArchiveTasked = errors.New("archival already tasked for this stream")

// ArchivalParams is the archival configuration.
type ArchivalParams struct {
	// RequestID is the unique request ID to use as a random base or the
	// archival key.
	RequestID string

	// SettleDelay is the amount of settle delay to attach to this request.
	SettleDelay time.Duration

	// CompletePeriod is the amount of time after the initial archival task is
	// executed when the task should fail if the stream is incomplete. After this
	// period has expired, the archival may complete successfully even if the
	// stream is missing log entries.
	CompletePeriod time.Duration

	// keyIndex is atomically incremented each time a request is published to
	// differentiate it from previous superfluous requests to the same stream.
	// This must be atomically-manipulated, since PublishTask may be called
	// multiple times for the same stream if executed as part of a transaction.
	keyIndex int32
}

// PublishTask creates and dispatches a task queue task for the supplied
// LogStream. PublishTask is goroutine-safe.
//
// This should be run within a transaction on ls. On success, ls's state will
// be updated to reflect the archival tasking.
//
// If the task is created successfully, this will return nil. If the LogStream
// already had a task dispatched, it will return ErrArchiveTasked.
func (p *ArchivalParams) PublishTask(c context.Context, ap ArchivalPublisher, ls *LogStream) error {
	if ls.State >= LSArchiveTasked {
		// An archival task has already been dispatched for this log stream.
		return ErrArchiveTasked
	}

	path := string(ls.Path())
	msg := logdog.ArchiveTask{
		Path: path,
		Key:  p.createArchivalKey(path),
	}
	if p.SettleDelay > 0 {
		msg.SettleDelay = google.NewDuration(p.SettleDelay)
	}
	if p.CompletePeriod > 0 {
		msg.CompletePeriod = google.NewDuration(p.CompletePeriod)
	}

	// Publish an archival request.
	if err := ap.Publish(c, &msg); err != nil {
		return err
	}

	// Update our LogStream's ArchiveState to reflect that an archival task has
	// been dispatched.
	ls.State = LSArchiveTasked
	ls.ArchivalKey = msg.Key
	return nil
}

// createArchivalKey returns a unique archival request key
func (p *ArchivalParams) createArchivalKey(path string) []byte {
	index := atomic.AddInt32(&p.keyIndex, 1)
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%d", p.RequestID, path, index)))
	return hash[:]
}

// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package output

import (
	"fmt"

	"github.com/luci/luci-go/common/proto/logdog/logpb"
)

// Output is a sink endpoint for groups of messages.
//
// An Output's methods must be goroutine-safe.
//
// Note that there is no guarantee that any of the bundles passed through an
// Output are ordered.
type Output interface {
	// SendBundle sends a constructed ButlerLogBundle through the Output.
	//
	// If an error is returned, it indicates a failure to send the bundle.
	// If there is a data error or a message type is not supported by the
	// Output, it should log the error and return nil.
	SendBundle(*logpb.ButlerLogBundle) error

	// MaxSize returns the maximum number of bytes that this Output can process
	// with a single send. A return value <=0 indicates that there si no fixed
	// maximum size for this Output.
	//
	// Since it is impossible for callers to know the actual size of the message
	// that is being submitted, and since message batching may cluster across
	// size boundaries, this should be a conservative estimate.
	MaxSize() int

	// Collect current Output stats.
	Stats() Stats

	// Close closes the Output, blocking until any buffered actions are flushed.
	Close()
}

// Stats is an interface to query Output statistics.
//
// An Output's ability to keep statistics varies with its implementation
// details. Currently, Stats are for debugging/information purposes only.
type Stats interface {
	fmt.Stringer

	// SentBytes returns the number of bytes
	SentBytes() int
	// SentMessages returns the number of successfully transmitted messages.
	SentMessages() int
	// DiscardedMessages returns the number of discarded messages.
	DiscardedMessages() int
	// Errors returns the number of errors encountered during operation.
	Errors() int
}

// StatsBase is a simple implementation of the Stats interface.
type StatsBase struct {
	F struct {
		SentBytes         int // The number of bytes sent.
		SentMessages      int // The number of messages sent.
		DiscardedMessages int // The number of messages that have been discarded.
		Errors            int // The number of errors encountered.
	}
}

var _ Stats = (*StatsBase)(nil)

func (s *StatsBase) String() string {
	return fmt.Sprintf("%+v", s.F)
}

// SentBytes implements Stats.
func (s *StatsBase) SentBytes() int {
	return s.F.SentBytes
}

// SentMessages implements Stats.
func (s *StatsBase) SentMessages() int {
	return s.F.SentMessages
}

// DiscardedMessages implements Stats.
func (s *StatsBase) DiscardedMessages() int {
	return s.F.DiscardedMessages
}

// Errors implements Stats.
func (s *StatsBase) Errors() int {
	return s.F.Errors
}

// Merge merges the values from one Stats block into another.
func (s *StatsBase) Merge(o Stats) {
	s.F.SentBytes += o.SentBytes()
	s.F.SentMessages += o.SentMessages()
	s.F.DiscardedMessages += o.DiscardedMessages()
	s.F.Errors += o.Errors()
}

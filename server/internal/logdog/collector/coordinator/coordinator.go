// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"fmt"

	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"
)

// Coordinator is an interface to a remote LogDog Coordinator service. This is
// a simplified version of the Coordinator's Service API tailored specifically
// to the Collector's usage.
//
// All Coordiantor methods will return transient-wrapped errors if appropriate.
type Coordinator interface {
	// RegisterStream registers a log stream state.
	RegisterStream(context.Context, *LogStreamState, *logpb.LogStreamDescriptor) (*LogStreamState, error)
	// TerminateStream registers the terminal index of a log stream state.
	TerminateStream(context.Context, *LogStreamState) error
}

// LogStreamState is a local representation of a remote stream's state. It is a
// subset of the remote state with the necessary elements for the Collector to
// operate and update.
type LogStreamState struct {
	Path          types.StreamPath   // Stream path.
	ProtoVersion  string             // Stream protocol version string.
	Secret        []byte             // Secret.
	TerminalIndex types.MessageIndex // Terminal index, <0 for unterminated.
	Archived      bool               // Is the stream archived?
	Purged        bool               // Is the stream purged?
}

type coordinatorImpl struct {
	c services.ServicesClient
}

// NewCoordinator returns a Coordinator implementation that uses a
// services.ServicesClient.
func NewCoordinator(s services.ServicesClient) Coordinator {
	return &coordinatorImpl{s}
}

func (*coordinatorImpl) clientSideValidate(s *LogStreamState) error {
	if err := s.Path.Validate(); err != nil {
		return err
	}
	if len(s.Secret) == 0 {
		return errors.New("missing stream secret")
	}
	return nil
}

func (c *coordinatorImpl) RegisterStream(ctx context.Context, s *LogStreamState, d *logpb.LogStreamDescriptor) (
	*LogStreamState, error) {
	if err := c.clientSideValidate(s); err != nil {
		return nil, err
	}
	if err := d.Validate(true); err != nil {
		return nil, fmt.Errorf("invalid descriptor: %s", err)
	}

	req := services.RegisterStreamRequest{
		Path:         string(s.Path),
		Secret:       s.Secret,
		ProtoVersion: s.ProtoVersion,
		Desc:         d,
	}

	resp, err := c.c.RegisterStream(ctx, &req)
	if err != nil {
		return nil, err
	}

	return &LogStreamState{
		Path:          types.StreamPath(resp.Path),
		ProtoVersion:  resp.ProtoVersion,
		Secret:        resp.Secret,
		TerminalIndex: types.MessageIndex(resp.TerminalIndex),
		Archived:      resp.Archived,
		Purged:        resp.Purged,
	}, nil
}

func (c *coordinatorImpl) TerminateStream(ctx context.Context, s *LogStreamState) error {
	if err := c.clientSideValidate(s); err != nil {
		return err
	}
	if s.TerminalIndex < 0 {
		return errors.New("refusing to terminate with non-terminal state")
	}

	req := services.TerminateStreamRequest{
		Path:          string(s.Path),
		Secret:        s.Secret,
		TerminalIndex: int64(s.TerminalIndex),
	}

	if _, err := c.c.TerminateStream(ctx, &req); err != nil {
		return err
	}
	return nil
}

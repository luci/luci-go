// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package coordinator

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/common/types"
)

// Coordinator is an interface to a remote LogDog Coordinator service. This is
// a simplified version of the Coordinator's Service API tailored specifically
// to the Collector's usage.
//
// All Coordinator methods will return transient-wrapped errors if appropriate.
type Coordinator interface {
	// RegisterStream registers a log stream state.
	RegisterStream(c context.Context, s *LogStreamState, desc []byte) (*LogStreamState, error)
	// TerminateStream registers the terminal index of a log stream state.
	TerminateStream(c context.Context, s *TerminateRequest) error
}

// LogStreamState is a local representation of a remote stream's state. It is a
// subset of the remote state with the necessary elements for the Collector to
// operate and update.
type LogStreamState struct {
	// Project is the log stream project.
	Project string
	// Path is the log stream path.
	Path types.StreamPath

	// ID is the stream's Coordinator ID. This is returned by the Coordinator.
	ID string
	// ProtoVersion is the stream protocol version string.
	ProtoVersion string
	// Secret is the log stream's prefix secret.
	Secret types.PrefixSecret
	// TerminalIndex is an optional terminal index to register alongside the
	// stream. If this is <0, the stream will be registered without a terminal
	// index.
	TerminalIndex types.MessageIndex

	// Archived is true if the log stream has been archived. This is returned by
	// the remote service.
	Archived bool
	// Purged is true if the log stream has been archived. This is returned by
	// the remote service.
	Purged bool
}

// TerminateRequest is a local representation of a Coordinator stream
// termination request.
type TerminateRequest struct {
	Project string           // Project name.
	Path    types.StreamPath // Stream path. Needed for cache lookup.

	// ID is the stream's Coordinator ID, as indicated by the Coordinator.
	ID string
	// TerminalIndex is the terminal index to register.
	//
	// This must be >= 0, else there is no point in sending the TerminateStream
	// request.
	TerminalIndex types.MessageIndex
	// Secret is the log stream's prefix secret.
	Secret types.PrefixSecret
}

type coordinatorImpl struct {
	c logdog.ServicesClient
}

// NewCoordinator returns a Coordinator implementation that uses a
// logdog.ServicesClient.
func NewCoordinator(s logdog.ServicesClient) Coordinator {
	return &coordinatorImpl{s}
}

func (c *coordinatorImpl) RegisterStream(ctx context.Context, s *LogStreamState, desc []byte) (*LogStreamState, error) {
	// Client-side validate our parameters.
	if err := config.ValidateProjectName(s.Project); err != nil {
		return nil, fmt.Errorf("failed to validate project: %s", err)
	}
	if err := s.Path.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate path: %s", err)
	}
	if err := s.Secret.Validate(); err != nil {
		return nil, fmt.Errorf("invalid secret: %s", err)
	}

	req := logdog.RegisterStreamRequest{
		Project:       string(s.Project),
		Secret:        []byte(s.Secret),
		ProtoVersion:  s.ProtoVersion,
		Desc:          desc,
		TerminalIndex: int64(s.TerminalIndex),
	}

	resp, err := c.c.RegisterStream(ctx, &req)
	switch {
	case err != nil:
		return nil, err
	case resp.State == nil:
		return nil, errors.New("missing stream state")
	}

	return &LogStreamState{
		ID:            resp.Id,
		Project:       s.Project,
		Path:          s.Path,
		ProtoVersion:  resp.State.ProtoVersion,
		Secret:        types.PrefixSecret(resp.State.Secret),
		TerminalIndex: types.MessageIndex(resp.State.TerminalIndex),
		Archived:      resp.State.Archived,
		Purged:        resp.State.Purged,
	}, nil
}

func (c *coordinatorImpl) TerminateStream(ctx context.Context, r *TerminateRequest) error {
	// Client-side validate our parameters.
	if err := config.ValidateProjectName(r.Project); err != nil {
		return fmt.Errorf("failed to validate project: %s", err)
	}
	if r.ID == "" {
		return errors.New("missing stream ID")
	}
	if r.TerminalIndex < 0 {
		return errors.New("refusing to terminate with non-terminal state")
	}

	req := logdog.TerminateStreamRequest{
		Project:       string(r.Project),
		Id:            r.ID,
		Secret:        []byte(r.Secret),
		TerminalIndex: int64(r.TerminalIndex),
	}

	if _, err := c.c.TerminateStream(ctx, &req); err != nil {
		return err
	}
	return nil
}

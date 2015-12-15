// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package swarming implements cron task that runs Swarming job.
package swarming

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	"github.com/luci/luci-go/appengine/cmd/cron/messages"
	"github.com/luci/luci-go/appengine/cmd/cron/task"
)

// TaskManager implements task.Manager interface for tasks defined with
// SwarmingTask proto message.
type TaskManager struct {
}

// Name is part of Manager interface.
func (m TaskManager) Name() string {
	return "swarming"
}

// ProtoMessageType is part of Manager interface.
func (m TaskManager) ProtoMessageType() proto.Message {
	return (*messages.SwarmingTask)(nil)
}

// ValidateProtoMessage is part of Manager interface.
func (m TaskManager) ValidateProtoMessage(msg proto.Message) error {
	cfg, ok := msg.(*messages.SwarmingTask)
	if !ok {
		return fmt.Errorf("wrong type %T, expecting *messages.SwarmingTask", msg)
	}

	// Validate 'server' field.
	server := cfg.GetServer()
	if server == "" {
		return fmt.Errorf("field 'server' is required")
	}
	u, err := url.Parse(server)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %s", server, err)
	}
	if !u.IsAbs() {
		return fmt.Errorf("not an absolute url: %q", server)
	}
	if u.Path != "" {
		return fmt.Errorf("not a host root url: %q", server)
	}

	// Validate environ, dimensions, tags.
	if err = validateKVList("environment variable", cfg.GetEnv(), '='); err != nil {
		return err
	}
	if err = validateKVList("dimension", cfg.GetDimensions(), ':'); err != nil {
		return err
	}
	if err = validateKVList("tag", cfg.GetTags(), ':'); err != nil {
		return err
	}

	// Validate priority.
	priority := cfg.GetPriority()
	if priority < 0 || priority > 255 {
		return fmt.Errorf("bad priority, must be [0, 255]: %d", priority)
	}

	// Can't have both 'command' and 'isolated_ref'.
	hasCommand := len(cfg.Command) != 0
	hasIsolatedRef := cfg.IsolatedRef != nil
	switch {
	case !hasCommand && !hasIsolatedRef:
		return fmt.Errorf("one of 'command' or 'isolated_ref' is required")
	case hasCommand && hasIsolatedRef:
		return fmt.Errorf("only one of 'command' or 'isolated_ref' must be specified, not both")
	}

	return nil
}

func validateKVList(kind string, list []string, sep rune) error {
	for _, item := range list {
		if strings.IndexRune(item, sep) == -1 {
			return fmt.Errorf("bad %s, not a 'key%svalue' pair: %q", kind, string(sep), item)
		}
	}
	return nil
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, msg proto.Message, ctl task.Controller, invNonce int64) error {
	// TODO(vadimsh): Implement.
	return ctl.Save(task.StatusSucceeded)
}

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return errors.New("not implemented")
}

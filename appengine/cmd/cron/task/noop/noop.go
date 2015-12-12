// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package noop implements cron task that does nothing at all. Used for testing
// only.
package noop

import (
	"errors"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/cron/messages"
	"github.com/luci/luci-go/appengine/cmd/cron/task"
)

// TaskManager implements task.Manager interface for tasks defined with
// NoopTask proto message.
type TaskManager struct {
}

// Name is part of Manager interface.
func (m TaskManager) Name() string {
	return "noop"
}

// ProtoMessageType is part of Manager interface.
func (m TaskManager) ProtoMessageType() proto.Message {
	return (*messages.NoopTask)(nil)
}

// ValidateProtoMessage is part of Manager interface.
func (m TaskManager) ValidateProtoMessage(msg proto.Message) error {
	return nil
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, msg proto.Message, ctl task.Controller) error {
	ctl.DebugLog("Running noop task")
	return ctl.Save(task.StatusSucceeded)
}

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller) error {
	return errors.New("not implemented")
}

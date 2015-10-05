// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package urlfetch implements cron tasks that just make HTTP call.
package urlfetch

import (
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/cron/messages"
	"github.com/luci/luci-go/appengine/cmd/cron/task"
)

// TaskManager implements task.Manager interface for tasks defined with
// UrlFetchTask proto message
type TaskManager struct {
}

// ProtoMessageType is part of Manager interface.
func (m TaskManager) ProtoMessageType() proto.Message {
	return &messages.UrlFetchTask{}
}

// ValidateProtoMessage is part of Manager interface.
func (m TaskManager) ValidateProtoMessage(msg proto.Message) error {
	return nil
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, msg proto.Message, ctl task.Controller) error {
	// TODO(vadimsh): Start the fetch for real.
	cfg := msg.(*messages.UrlFetchTask)
	ctl.DebugLog("%s %s", cfg.GetMethod(), cfg.GetUrl())
	if err := ctl.Save(task.StatusRunning); err != nil {
		return err
	}
	time.Sleep(1 * time.Second)
	ctl.DebugLog("Success!")
	return ctl.Save(task.StatusSucceeded)
}

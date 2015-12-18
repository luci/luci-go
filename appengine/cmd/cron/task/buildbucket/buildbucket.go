// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package buildbucket implements cron task that runs Buildbucket jobs.
package buildbucket

import (
	"errors"
	"fmt"
	"net/url"

	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	"github.com/golang/protobuf/proto"

	"github.com/luci/luci-go/appengine/cmd/cron/messages"
	"github.com/luci/luci-go/appengine/cmd/cron/task"
	"github.com/luci/luci-go/appengine/cmd/cron/task/utils"
)

// TaskManager implements task.Manager interface for tasks defined with
// BuildbucketTask proto message.
type TaskManager struct {
}

// Name is part of Manager interface.
func (m TaskManager) Name() string {
	return "buildbucket"
}

// ProtoMessageType is part of Manager interface.
func (m TaskManager) ProtoMessageType() proto.Message {
	return (*messages.BuildbucketTask)(nil)
}

// ValidateProtoMessage is part of Manager interface.
func (m TaskManager) ValidateProtoMessage(msg proto.Message) error {
	cfg, ok := msg.(*messages.BuildbucketTask)
	if !ok {
		return fmt.Errorf("wrong type %T, expecting *messages.BuildbucketTask", msg)
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

	// Bucket and builder fields are required.
	if cfg.GetBucket() == "" {
		return fmt.Errorf("'bucket' field is required")
	}
	if cfg.GetBuilder() == "" {
		return fmt.Errorf("'builder' field is required")
	}

	// Validate 'properties' and 'tags'.
	if err = utils.ValidateKVList("property", cfg.GetProperties(), ':'); err != nil {
		return err
	}
	if err = utils.ValidateKVList("tag", cfg.GetTags(), ':'); err != nil {
		return err
	}

	return nil
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	return errors.New("not implemented yet")
}

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return errors.New("not implemented yet")
}

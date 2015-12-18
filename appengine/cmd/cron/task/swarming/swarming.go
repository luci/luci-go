// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package swarming implements cron task that runs Swarming job.
package swarming

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/pubsub/v1"

	"github.com/luci/luci-go/appengine/cmd/cron/messages"
	"github.com/luci/luci-go/appengine/cmd/cron/task"
	"github.com/luci/luci-go/appengine/cmd/cron/task/utils"
	"github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/errors"
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
	if err = utils.ValidateKVList("environment variable", cfg.GetEnv(), '='); err != nil {
		return err
	}
	if err = utils.ValidateKVList("dimension", cfg.GetDimensions(), ':'); err != nil {
		return err
	}
	if err = utils.ValidateKVList("tag", cfg.GetTags(), ':'); err != nil {
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

func kvListToStringPairs(list []string, sep rune) (out []*swarming.SwarmingRpcsStringPair) {
	for _, pair := range utils.UnpackKVList(list, sep) {
		out = append(out, &swarming.SwarmingRpcsStringPair{
			Key:   pair.Key,
			Value: pair.Value,
		})
	}
	return out
}

// defaultExpirationTimeout derives Swarming queuing timeout: max time a task
// is kept in the queue (not being picked up by bots), before it is marked as
// failed.
func defaultExpirationTimeout(ctl task.Controller) time.Duration {
	// TODO(vadimsh): Do something smarter, e.g. look at next expected invocation
	// time.
	return 30 * time.Minute
}

// defaultExecutionTimeout derives hard deadline for a task if it wasn't
// explicitly specified in the config.
func defaultExecutionTimeout(ctl task.Controller) time.Duration {
	// TODO(vadimsh): Do something smarter, e.g. look at next expected invocation
	// time.
	return time.Hour
}

// isTransientAPIError returns true if error from Google API client indicates
// an error that can go away on its own in the future.
func isTransientAPIError(err error) bool {
	if err == nil {
		return false
	}
	apiErr, _ := err.(*googleapi.Error)
	if apiErr == nil {
		return true // failed to get HTTP code => connectivity error => transient
	}
	return apiErr.Code >= 500 || apiErr.Code == 429
}

// taskData is saved in Invocation.TaskData field.
type taskData struct {
	SwarmingTaskID string `json:"swarming_task_id"`
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	// At this point config is already validated by ValidateProtoMessage.
	cfg := ctl.Task().(*messages.SwarmingTask)

	// Default set of tags.
	tags := []string{"cron:" + ctl.JobID()}
	tags = append(tags, cfg.Tags...)

	// How long to keep a task in swarming queue (not running) before marking it
	// as expired.
	expirationSecs := int64(defaultExpirationTimeout(ctl) / time.Second)

	// The hard deadline: how long task can run once it has started.
	executionTimeoutSecs := int64(cfg.GetExecutionTimeoutSecs())
	if executionTimeoutSecs == 0 {
		executionTimeoutSecs = int64(defaultExecutionTimeout(ctl) / time.Second)
	}

	// Make sure Swarming can publish PubSub messages, grab token that would
	// identify this invocation when receiving PubSub notifications.
	ctl.DebugLog("Preparing PubSub topic for %q", *cfg.Server)
	topic, authToken, err := ctl.PrepareTopic(*cfg.Server)
	if err != nil {
		ctl.DebugLog("Failed to prepare PubSub topic - %s", err)
		return err
	}
	ctl.DebugLog("PubSub topic is %q", topic)

	// Prepare the request.
	request := swarming.SwarmingRpcsNewTaskRequest{
		Name:            fmt.Sprintf("cron:%s/%d", ctl.JobID(), ctl.InvocationID()),
		ExpirationSecs:  expirationSecs,
		Priority:        int64(cfg.GetPriority()),
		PubsubAuthToken: "...", // set a bit later, after printing this struct
		PubsubTopic:     topic,
		Tags:            tags,
		Properties: &swarming.SwarmingRpcsTaskProperties{
			Dimensions:           kvListToStringPairs(cfg.Dimensions, ':'),
			Env:                  kvListToStringPairs(cfg.Env, '='),
			ExecutionTimeoutSecs: executionTimeoutSecs,
			ExtraArgs:            cfg.ExtraArgs,
			GracePeriodSecs:      int64(cfg.GetGracePeriodSecs()),
			Idempotent:           false,
			IoTimeoutSecs:        int64(cfg.GetIoTimeoutSecs()),
		},
	}

	// Only one of InputsRef or Command must be set.
	if cfg.IsolatedRef != nil {
		request.Properties.InputsRef = &swarming.SwarmingRpcsFilesRef{
			Isolated:       cfg.IsolatedRef.GetIsolated(),
			Isolatedserver: cfg.IsolatedRef.GetIsolatedServer(),
			Namespace:      cfg.IsolatedRef.GetNamespace(),
		}
	} else {
		request.Properties.Command = cfg.Command
	}

	// Serialize for debug log without auth token.
	blob, err := json.MarshalIndent(&request, "", "  ")
	if err != nil {
		return err
	}
	ctl.DebugLog("Swarming task request:\n%s", string(blob))
	request.PubsubAuthToken = authToken // can put the token now

	// Trigger the task.
	service, err := m.createSwarmingService(c, ctl)
	if err != nil {
		return err
	}
	resp, err := service.Tasks.New(&request).Context(c).Do()
	if err != nil {
		ctl.DebugLog("Failed to trigger the task - %s", err)
		if isTransientAPIError(err) {
			return errors.WrapTransient(err)
		}
		return err
	}

	// Dump response in full to the debug log. It doesn't contain any secrets.
	blob, err = json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	ctl.DebugLog("Swarming response:\n%s", string(blob))

	// Save TaskId in invocation, will be used later when handling PubSub
	// notifications
	ctl.State().TaskData, err = json.Marshal(&taskData{
		SwarmingTaskID: resp.TaskId,
	})
	if err != nil {
		return err
	}

	// Successfully launched.
	ctl.State().Status = task.StatusRunning
	ctl.State().ViewURL = fmt.Sprintf("%s/user/task/%s", *cfg.Server, resp.TaskId)
	ctl.DebugLog("Task URL: %s", ctl.State().ViewURL)

	// Maybe the task was already finished? Can only happen when 'idempotent' is
	// set to true (which we don't do currently), but handle this case here for
	// completeness anyway.
	if resp.TaskResult != nil {
		ctl.DebugLog("Task request was deduplicated")
		m.handleTaskResult(c, ctl, resp.TaskResult)
	}
	return nil
}

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	switch status := ctl.State().Status; {
	// This can happen if Swarming manages to send PubSub message before
	// LaunchTask finishes. Do not touch State or DebugLog to avoid collision with
	// still running LaunchTask when saving the invocation, it will only make the
	// matters worse.
	case status == task.StatusStarting:
		return errors.WrapTransient(errors.New("invocation is still starting, try again later"))
	case status != task.StatusRunning:
		return fmt.Errorf("unexpected invocation status %q, expecting %q", status, task.StatusRunning)
	}

	// Grab task ID from the blob generated in LaunchTask.
	taskData := taskData{}
	if err := json.Unmarshal(ctl.State().TaskData, &taskData); err != nil {
		ctl.State().Status = task.StatusFailed
		return fmt.Errorf("could not parse TaskData - %s", err)
	}

	// Fetch task result from Swarming.
	ctl.DebugLog("Received PubSub notification, asking swarming for the task status")
	service, err := m.createSwarmingService(c, ctl)
	if err != nil {
		return err
	}
	resp, err := service.Task.Result(taskData.SwarmingTaskID).Context(c).Do()
	if err != nil {
		ctl.DebugLog("Failed to fetch task results - %s", err)
		if isTransientAPIError(err) {
			err = errors.WrapTransient(err)
		} else {
			ctl.State().Status = task.StatusFailed
		}
		return err
	}
	m.handleTaskResult(c, ctl, resp)
	return nil
}

// createSwarmingService makes a configured Swarming API client.
func (m TaskManager) createSwarmingService(c context.Context, ctl task.Controller) (*swarming.Service, error) {
	// TODO(vadimsh): Use per-project service accounts, not a global cron service
	// account.
	cfg := ctl.Task().(*messages.SwarmingTask)
	var transport http.RoundTripper
	if strings.HasPrefix(*cfg.Server, "http://") {
		transport = http.DefaultTransport // local tests
	} else {
		var err error
		if transport, err = client.Transport(c, nil, nil); err != nil {
			return nil, err
		}
	}
	service, err := swarming.New(&http.Client{Transport: transport})
	if err != nil {
		return nil, err
	}
	service.BasePath = *cfg.Server + "/_ah/api/swarming/v1/"
	return service, nil
}

// handleTaskResult processes swarming task results message updating the state
// of the invocation.
func (m TaskManager) handleTaskResult(c context.Context, ctl task.Controller, r *swarming.SwarmingRpcsTaskResult) {
	ctl.DebugLog(
		"The task is in state %q (failure: %v, internalFailure: %v)",
		r.State, r.Failure, r.InternalFailure)
	switch {
	case r.State == "PENDING" || r.State == "RUNNING":
		return // do nothing
	case r.State == "COMPLETED" && !(r.Failure || r.InternalFailure):
		ctl.State().Status = task.StatusSucceeded
	default:
		ctl.State().Status = task.StatusFailed
	}
}

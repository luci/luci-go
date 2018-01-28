// Copyright 2015 The LUCI Authors.
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

// Package buildbucket implements tasks that run Buildbucket jobs.
package buildbucket

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/buildbucket"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils"
)

const (
	statusCheckTimerName     = "check-buildbucket-build-status"
	statusCheckTimerInterval = time.Minute
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

// Traits is part of Manager interface.
func (m TaskManager) Traits() task.Traits {
	return task.Traits{
		Multistage: true, // we use task.StatusRunning state
	}
}

// ValidateProtoMessage is part of Manager interface.
func (m TaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message) {
	cfg, ok := msg.(*messages.BuildbucketTask)
	if !ok {
		c.Errorf("wrong type %T, expecting *messages.BuildbucketTask", msg)
		return
	}
	if cfg == nil {
		c.Errorf("expecting a non-empty BuildbucketTask")
		return
	}

	// Validate 'server' field.
	switch {
	case cfg.Server == "":
		c.Errorf("field 'server' is required")
	case strings.HasPrefix(cfg.Server, "https://") || strings.HasPrefix(cfg.Server, "http://"):
		c.Errorf("field 'server' should be just a host, not a URL: %q", cfg.Server)
	default:
		u, err := url.Parse("https://" + cfg.Server)
		switch {
		case err != nil:
			c.Errorf("field 'server' is not a valid hostname %q: %s", cfg.Server, err)
		case !u.IsAbs() || u.Path != "":
			c.Errorf("field 'server' is not a valid hostname %q", cfg.Server)
		}
	}

	// Bucket and builder fields are required.
	if cfg.Bucket == "" {
		c.Errorf("'bucket' field is required")
	}
	if cfg.Builder == "" {
		c.Errorf("'builder' field is required")
	}

	// Validate 'properties' and 'tags'.
	if err := utils.ValidateKVList("property", cfg.Properties, ':'); err != nil {
		c.Enter("properties")
		c.Error(err)
		c.Exit()
	}
	if err := utils.ValidateKVList("tag", cfg.Tags, ':'); err != nil {
		c.Enter("tags")
		c.Error(err)
		c.Exit()
		return
	}
	// Default tags can not be overridden.
	defTags := defaultTags(nil, nil, nil)
	for _, kv := range utils.UnpackKVList(cfg.Tags, ':') {
		if _, ok := defTags[kv.Key]; ok {
			c.Errorf("tag %q is reserved", kv.Key)
		}
	}
}

// defaultTags returns map with default set of tags.
//
// If context is nil, only keys are set.
func defaultTags(c context.Context, ctl task.Controller, cfg *messages.BuildbucketTask) map[string]string {
	if c != nil {
		return map[string]string{
			"builder":                 cfg.Builder,
			"scheduler_invocation_id": fmt.Sprintf("%d", ctl.InvocationID()),
			"scheduler_job_id":        ctl.JobID(),
			"user_agent":              info.AppID(c),
		}
	}
	return map[string]string{
		"builder":                 "",
		"scheduler_invocation_id": "",
		"scheduler_job_id":        "",
		"user_agent":              "",
	}
}

// taskData is saved in Invocation.TaskData field.
type taskData struct {
	BuildID int64 `json:"build_id,omitempty,string"`
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	cfg := ctl.Task().(*messages.BuildbucketTask) // already validated
	req := ctl.Request()

	// Join tags from all known sources. Note: no overriding here for now, tags
	// with identical keys are allowed.
	tags := utils.KVListFromMap(defaultTags(c, ctl, cfg)).Pack(':')
	tags = append(tags, cfg.Tags...)
	tags = append(tags, req.Tags...)

	// Prepare properties for the build. Properties from the request override the
	// ones in the config.
	props := &structpb.Struct{
		Fields: make(map[string]*structpb.Value, len(cfg.Properties)+len(req.Properties.GetFields())),
	}
	for _, kv := range utils.UnpackKVList(cfg.Properties, ':') {
		props.Fields[kv.Key] = strProtoValue(kv.Value)
	}
	for k, v := range req.Properties.GetFields() {
		props.Fields[k] = v
	}

	// TODO(vadimsh): Remove this fallback once all servers are updated to supply
	// req.Properties for gitiles triggers. Invocation triggered by old server
	// code supply list of triggers, but no Properties or Tags. So build them
	// right here. This is needed only to support in-flight invocations started
	// when the server is being updated.
	if req.Properties == nil {
		if t := maybeGetTrigger(c, ctl); t != nil {
			props.Fields["revision"] = strProtoValue(t.revision)
			props.Fields["branch"] = strProtoValue(t.ref)
			tags = append(tags, "buildset:"+t.buildset(), "gitiles_ref:"+t.ref)
		}
	}

	// Prepare JSON blob for Buildbucket. encoding/json and jsonpb doesn't
	// interoperate with each other, so stick with jsonpb for this.
	paramsJSON, err := (&jsonpb.Marshaler{}).MarshalToString(&structpb.Struct{
		Fields: map[string]*structpb.Value{
			"builder_name": strProtoValue(cfg.Builder),
			"properties": {
				Kind: &structpb.Value_StructValue{
					StructValue: props,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal parameters JSON - %s", err)
	}

	// Make sure Buildbucket can publish PubSub messages, grab token that would
	// identify this invocation when receiving PubSub notifications.
	serverURL := makeServerUrl(cfg.Server)
	ctl.DebugLog("Preparing PubSub topic for %q", serverURL)
	topic, authToken, err := ctl.PrepareTopic(c, serverURL)
	if err != nil {
		ctl.DebugLog("Failed to prepare PubSub topic - %s", err)
		return err
	}
	ctl.DebugLog("PubSub topic is %q", topic)

	// Prepare the request.
	request := bbapi.ApiPutRequestMessage{
		Bucket:            cfg.Bucket,
		ClientOperationId: fmt.Sprintf("%d", ctl.InvocationNonce()),
		ParametersJson:    paramsJSON,
		Tags:              tags,
		PubsubCallback: &bbapi.ApiPubSubCallbackMessage{
			AuthToken: "...", // set a bit later, after printing this struct
			Topic:     topic,
		},
	}

	// Serialize for debug log without auth token.
	blob, err := json.MarshalIndent(&request, "", "  ")
	if err != nil {
		return err
	}
	ctl.DebugLog("Buildbucket request:\n%s", blob)
	request.PubsubCallback.AuthToken = authToken // can put the token now

	// The next call may take a while. Dump the current log to the datastore.
	// Ignore errors here, it is best effort attempt to update the log.
	ctl.Save(c)

	// Send the request.
	service, err := m.createBuildbucketService(c, ctl)
	if err != nil {
		return err
	}
	resp, err := service.Put(&request).Do()
	if err != nil {
		ctl.DebugLog("Failed to add buildbucket build - %s", err)
		return utils.WrapAPIError(err)
	}

	// Dump response in full to the debug log. It doesn't contain any secrets.
	blob, err = json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	ctl.DebugLog("Buildbucket response:\n%s", blob)

	if resp.Error != nil {
		ctl.DebugLog("Buildbucket returned error %q: %s", resp.Error.Reason, resp.Error.Message)
		return fmt.Errorf("buildbucket error %q - %s", resp.Error.Reason, resp.Error.Message)
	}
	if resp.Build == nil {
		ctl.DebugLog("Buildbucket response is not valid, missing 'build'")
		return fmt.Errorf("bad buildbucket response, no 'build' field")
	}

	// Save build id in invocation, will be used later when handling PubSub
	// notifications
	ctl.State().TaskData, err = json.Marshal(&taskData{BuildID: resp.Build.Id})
	if err != nil {
		return err
	}

	// Successfully launched.
	ctl.State().Status = task.StatusRunning
	ctl.State().ViewURL = resp.Build.Url
	ctl.DebugLog("Task URL: %s", ctl.State().ViewURL)

	// Maybe finished already? It can happen if we are retrying a call with same
	// ClientOperationId as a finished one.
	if resp.Build.Status == "COMPLETED" {
		m.handleBuildResult(c, ctl, resp.Build)
	}

	// This will schedule status check if the task is actually running.
	m.checkBuildStatusLater(c, ctl)
	return nil
}

// AbortTask is part of Manager interface.
func (m TaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	// TODO(vadimsh): Send the abort signal to buildbucket.
	return nil
}

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	ctl.DebugLog("Received PubSub notification, asking Buildbucket for the build status")
	return m.checkBuildStatus(c, ctl)
}

// HandleTimer is part of Manager interface.
func (m TaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	if name == statusCheckTimerName {
		ctl.DebugLog("Timer tick, asking Buildbucket for the build status")
		if err := m.checkBuildStatus(c, ctl); err != nil {
			// This is either a fatal or transient error. If it is fatal, no need to
			// schedule the timer anymore. If it is transient, HandleTimer call itself
			// will be retried and the timer when be rescheduled then.
			return err
		}
		m.checkBuildStatusLater(c, ctl) // reschedule this check
	}
	return nil
}

func makeServerUrl(s string) string {
	if strings.HasPrefix(s, "http://") {
		// Used only in tests where we hardcode http in cfg.Server because local server is http not https.
		return s
	}
	return "https://" + s
}

// createBuildbucketService makes a configured Buildbucket API client.
func (m TaskManager) createBuildbucketService(c context.Context, ctl task.Controller) (*bbapi.Service, error) {
	client, err := ctl.GetClient(c, time.Minute)
	if err != nil {
		return nil, err
	}
	service, err := bbapi.New(client)
	if err != nil {
		return nil, err
	}
	cfg := ctl.Task().(*messages.BuildbucketTask)
	service.BasePath = makeServerUrl(cfg.Server) + "/_ah/api/buildbucket/v1/"
	return service, nil
}

// checkBuildStatusLater schedules a delayed call to checkBuildStatus if the
// invocation is still running.
//
// This is a fallback mechanism in case PubSub notifications are delayed or
// lost for some reason.
func (m TaskManager) checkBuildStatusLater(c context.Context, ctl task.Controller) {
	// TODO(vadimsh): Make the check interval configurable?
	if !ctl.State().Status.Final() {
		ctl.AddTimer(c, statusCheckTimerInterval, statusCheckTimerName, nil)
	}
}

func (m TaskManager) checkBuildStatus(c context.Context, ctl task.Controller) error {
	switch status := ctl.State().Status; {
	// This can happen if Buildbucket manages to send PubSub message before
	// LaunchTask finishes. Do not touch State or DebugLog to avoid collision with
	// still running LaunchTask when saving the invocation, it will only make the
	// matters worse.
	case status == task.StatusStarting:
		return errors.New("invocation is still starting, try again later", transient.Tag)
	case status != task.StatusRunning:
		return fmt.Errorf("unexpected invocation status %q, expecting %q", status, task.StatusRunning)
	}

	// Grab build ID from the blob generated in LaunchTask.
	taskData := taskData{}
	if err := json.Unmarshal(ctl.State().TaskData, &taskData); err != nil {
		ctl.State().Status = task.StatusFailed
		return fmt.Errorf("could not parse TaskData - %s", err)
	}

	// Fetch build result from buildbucket.
	service, err := m.createBuildbucketService(c, ctl)
	if err != nil {
		return err
	}
	resp, err := service.Get(taskData.BuildID).Do()
	if err != nil {
		ctl.DebugLog("Failed to fetch buildbucket build - %s", err)
		err = utils.WrapAPIError(err)
		if !transient.Tag.In(err) {
			ctl.State().Status = task.StatusFailed
		}
		return err
	}

	// Dump response in full to the debug log. It doesn't contain any secrets.
	blob, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}

	// Give up (fail invocation) on unexpected fatal errors.
	if resp.Error != nil {
		ctl.DebugLog("Buildbucket returned error %q: %s", resp.Error.Reason, resp.Error.Message)
		ctl.DebugLog("Buildbucket response:\n%s", blob)
		ctl.State().Status = task.StatusFailed
		return fmt.Errorf("buildbucket error %q - %s", resp.Error.Reason, resp.Error.Message)
	}
	if resp.Build == nil {
		ctl.DebugLog("Buildbucket response is not valid, missing 'build'")
		ctl.DebugLog("Buildbucket response:\n%s", blob)
		ctl.State().Status = task.StatusFailed
		return fmt.Errorf("bad buildbucket response, no 'build' field")
	}

	m.handleBuildResult(c, ctl, resp.Build)
	if ctl.State().Status.Final() {
		ctl.DebugLog("Buildbucket build:\n%s", blob)
	}

	return nil
}

// handleBuildResult processes buildbucket results message updating the state
// of the invocation.
func (m TaskManager) handleBuildResult(c context.Context, ctl task.Controller, r *bbapi.ApiCommonBuildMessage) {
	ctl.DebugLog(
		"Build %d: status %q, result %q, failure_reason %q, cancelation_reason %q",
		r.Id, r.Status, r.Result, r.FailureReason, r.CancelationReason)
	switch {
	case r.Status == "SCHEDULED" || r.Status == "STARTED":
		return // do nothing
	case r.Status == "COMPLETED" && r.Result == "SUCCESS":
		ctl.State().Status = task.StatusSucceeded
	default:
		ctl.State().Status = task.StatusFailed
	}
}

func strProtoValue(s string) *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: s,
		},
	}
}

// TODO(vadimsh): Remove this code once the fallback in LaunchTask is removed.
// This code now lives in engine/request.go.

func maybeGetTrigger(c context.Context, ctl task.Controller) *normalizedTrigger {
	var prevTriggerID string
	var latest *normalizedTrigger
	for _, t := range ctl.Request().IncomingTriggers {
		g := t.GetGitiles()
		if g == nil {
			ctl.DebugLog("ignoring non-gitiles trigger %s", t.Id)
			continue
		}
		n, err := normalizeGitilesTrigger(g)
		if err != nil {
			logging.Errorf(c, "failed to normalize GitilesTrigger id='%s': %s", t.Id, err)
			ctl.DebugLog("failed to normalize GitilesTrigger id='%s': %s", t.Id, err)
			continue
		}
		// Latest trigger wins until engine v2 is available, because v1 requires
		// coalescing all pending triggers into just 1 invocation.
		if latest != nil {
			ctl.DebugLog("ignoring gitiles trigger %s", prevTriggerID)
		}
		latest = n
		prevTriggerID = t.Id
	}
	return latest
}

type normalizedTrigger struct {
	repo     *url.URL
	ref      string
	revision string
}

func (n *normalizedTrigger) buildset() string {
	c := buildbucket.GitilesCommit{
		Host:     n.repo.Host,
		Project:  strings.TrimPrefix(n.repo.Path, "/"),
		Revision: n.revision,
	}
	return c.String()
}

func normalizeGitilesTrigger(in *api.GitilesTrigger) (*normalizedTrigger, error) {
	u, err := gitiles.NormalizeRepoURL(in.Repo, false)
	if err != nil {
		return nil, err
	}
	return &normalizedTrigger{repo: u, ref: in.Ref, revision: in.Revision}, nil
}

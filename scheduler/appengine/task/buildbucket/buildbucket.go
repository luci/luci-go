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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/pubsub/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/appengine/tq"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbgrpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils"
)

const (
	// Parameters of a periodic build status check timer.
	statusCheckTimerName        = "check-buildbucket-build-status"
	statusCheckTimerIntervalMin = time.Minute
	statusCheckTimerIntervalMax = 10 * time.Minute

	// Maximum number of triggers to be emitted into $recipe_engine/scheduler
	// property. See also https://crbug.com/1006914.
	maxTriggersAsSchedulerProperty = 100
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
func (m TaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message, realmID string) {
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

	// Check can derive the bucket name.
	if _, err := builderID(cfg, realmID); err != nil {
		c.Errorf("%s", err)
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
	defTags := defaultTags(context.Background(), nil)
	for _, kv := range utils.UnpackKVList(cfg.Tags, ':') {
		if _, ok := defTags[kv.Key]; ok {
			c.Errorf("tag %q is reserved", kv.Key)
		}
	}
}

// defaultTags returns map with default set of tags.
//
// If context does not contain service info, only keys are set and the values are empty strings.
//
// The map returned by this function will always contain precisely the following keys:
// 1) "scheduler_invocation_id"
// 2) "scheduler_job_id"
// 3) "user_agent"
func defaultTags(ctx context.Context, ctl task.Controller) map[string]string {
	goodCtx := info.Raw(ctx) != nil
	if goodCtx {
		return map[string]string{
			"scheduler_invocation_id": fmt.Sprintf("%d", ctl.InvocationID()),
			"scheduler_job_id":        ctl.JobID(),
			"user_agent":              info.AppID(ctx),
		}
	}
	return map[string]string{
		"scheduler_invocation_id": "",
		"scheduler_job_id":        "",
		"user_agent":              "",
	}
}

// taskData is saved in Invocation.TaskData field.
type taskData struct {
	BuildID int64 `json:"build_id,omitempty,string"`
}

// writeTaskData puts information about the task into invocation's TaskData.
func writeTaskData(ctl task.Controller, td *taskData) (err error) {
	if ctl.State().TaskData, err = json.Marshal(td); err != nil {
		return errors.Fmt("could not serialize TaskData: %w", err)
	}
	return nil
}

// readTaskData parses task data blob as prepared by writeTaskData.
func readTaskData(ctl task.Controller) (*taskData, error) {
	td := &taskData{}
	if err := json.Unmarshal(ctl.State().TaskData, td); err != nil {
		return nil, errors.Fmt("could not parse TaskData: %w", err)
	}
	return td, nil
}

// LaunchTask is part of Manager interface.
func (m TaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	cfg := ctl.Task().(*messages.BuildbucketTask) // already validated
	req := ctl.Request()

	// Generate full builder ID from the config. It should succeed since the
	// config has been validated already.
	bid, err := builderID(cfg, ctl.RealmID())
	if err != nil {
		return errors.Fmt("unexpected bad bucket name in the task config: %w", err)
	}

	// Join tags from all known sources. Note: no overriding here for now, tags
	// with identical keys are allowed.
	tags := utils.KVListFromMap(defaultTags(c, ctl)).Pack(':')
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

	// TODO(crbug.com/981945, crbug.com/939368): re-enable in chromium
	if bid.Project != "chromium" && bid.Project != "chrome" {
		var err error
		if props.Fields["$recipe_engine/scheduler"], err = schedulerProperty(c, ctl); err != nil {
			return fmt.Errorf("failed to generate scheduled property - %s", err)
		}
	}

	// Extract GitilesCommit from the most recent trigger, if possible.
	var commit *bbpb.GitilesCommit
	if last := req.LastTrigger(); last != nil {
		if gt := last.GetGitiles(); gt != nil {
			commit, err = triggerToCommit(gt)
			if err != nil {
				return errors.Fmt("failed to prepare gitiles_commit: %w", err)
			}
		}
	}

	// Process properties and tags that were used in Buildbucket v1 API, but now
	// are forbidden in Buildbucket v2 API in favor of GitilesCommit. Some LUCI
	// Scheduler users still pass them via EmitTriggers.
	switch commitFromTags := popCommitFromTags(ctl, props, &tags); {
	case commit == nil && commitFromTags != nil:
		ctl.DebugLog("Reconstructed gitiles commit from tags")
		commit = commitFromTags
	case commit != nil && commitFromTags != nil:
		if proto.Equal(commit, commitFromTags) {
			ctl.DebugLog("Popped gitiles commit info from properties and tags")
		} else {
			ctl.DebugLog("crbug.com/1182002: Gitiles commit from triggers doesn't match the one from tags")
			ctl.DebugLog("From triggers:\n%s", protoToJSON(commit))
			ctl.DebugLog("From properties:\n%s", protoToJSON(commitFromTags))
			ctl.DebugLog("Using the one from tags")
			commit = commitFromTags // to match pre-v2 logic
		}
	}

	// Make sure Buildbucket can publish PubSub messages, grab the token that
	// would identify this invocation when receiving PubSub notifications.
	serverURL := makeServerURL(cfg.Server)
	ctl.DebugLog("Preparing PubSub topic for %q", serverURL)
	topic, authToken, err := ctl.PrepareTopic(c, serverURL)
	if err != nil {
		ctl.DebugLog("Failed to prepare PubSub topic - %s", err)
		return err
	}
	ctl.DebugLog("PubSub topic is %q", topic)

	// Prepare the request.
	request := &bbpb.ScheduleBuildRequest{
		RequestId:     fmt.Sprintf("%d", ctl.InvocationID()),
		Builder:       bid,
		Properties:    props,
		GitilesCommit: commit,
		Tags:          toBuildbucketPairs(tags),
		Notify: &bbpb.NotificationConfig{
			PubsubTopic: topic,
			UserData:    nil, // set a bit later, after printing this struct
		},
	}

	// Serialize for debug log without PubSub auth token.
	ctl.DebugLog("Buildbucket request:\n%s", protoToJSON(request))
	request.Notify.UserData = []byte(authToken) // can put the token now

	// The next call may take a while. Dump the current log to the datastore.
	// Ignore errors here, it is best effort attempt to update the log.
	ctl.Save(c)

	// Send the request.
	var build *bbpb.Build
	err = m.withBuildbucket(c, ctl, func(ctx context.Context, bb bbgrpcpb.BuildsClient) (err error) {
		build, err = bb.ScheduleBuild(ctx, request)
		return
	})
	if err != nil {
		ctl.DebugLog("Failed to schedule Buildbucket build - %s", err)
		return grpcutil.WrapIfTransientOr(err, codes.DeadlineExceeded)
	}

	// Dump the response in full to the debug log. It doesn't contain any secrets.
	ctl.DebugLog("Scheduled build:\n%s", protoToJSON(build))

	// Save the build ID in the invocation, will be used later to make RPCs to
	// Buildbucket to check build's status.
	if err := writeTaskData(ctl, &taskData{BuildID: build.Id}); err != nil {
		return err
	}

	// Successfully launched.
	ctl.State().Status = task.StatusRunning
	ctl.State().ViewURL = fmt.Sprintf("%s/build/%d", serverURL, build.Id)
	ctl.DebugLog("Task URL: %s", ctl.State().ViewURL)

	// Check if maybe finished already? It can happen if we are retrying the call
	// with the same RequestId as a finished one.
	handleBuildStatus(ctl, build)

	// This will schedule status check if the task is actually running.
	m.checkBuildStatusLater(c, ctl)
	return nil
}

// AbortTask is part of Manager interface.
func (m TaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	// This can happen if the invocation is aborted before it has even started.
	// We don't have buildbucket build ID yet to cancel.
	//
	// There's a high chance that LaunchTask is executing concurrently somewhere.
	// We let it finish peacefully, by not touching the invocation state at all
	// and failing with a transient error instead. This avoids a collision on
	// State modification. When such collision happens, results of LaunchTask
	// (including a fresh build ID) are discarded (as the engine is unable to
	// "merge" conflicting mutations from two different state transitions). This
	// is really bad, since this process produces orphaned Buildbucket builds.
	//
	// So we pick a lesser evil and make AbortTask fail transiently while
	// invocation is starting.
	if status := ctl.State().Status; status.Initial() {
		return transient.Tag.Apply(errors.

			// Grab build ID from the blob generated in LaunchTask.
			Fmt("can't abort Buildbucket invocation in state %q", status))
	}

	taskData, err := readTaskData(ctl)
	if err != nil {
		ctl.State().Status = task.StatusFailed
		return err
	}

	// Ask Buildbucket to cancel this build.
	err = m.withBuildbucket(c, ctl, func(ctx context.Context, bb bbgrpcpb.BuildsClient) error {
		_, err := bb.CancelBuild(ctx, &bbpb.CancelBuildRequest{
			Id:              taskData.BuildID,
			SummaryMarkdown: "Canceled via LUCI Scheduler",
		})
		return err
	})
	return grpcutil.WrapIfTransientOr(err, codes.DeadlineExceeded)
}

// ExamineNotification is part of Manager interface.
func (m TaskManager) ExamineNotification(c context.Context, msg *pubsub.PubsubMessage) string {
	// Buildbucket v1 builds have the token in attributes.
	if tok := msg.Attributes["auth_token"]; tok != "" {
		return tok
	}
	// Buildbucket v2 builds have the token as "user_data" in the JSON message
	// body. The message body itself is base64-encoded.
	blob, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		logging.Warningf(c, "PubSub message data is not base64: %s", err)
		return ""
	}

	var body struct {
		UserDataLegacy string `json:"user_data,omitempty"`
		UserData       []byte `json:"userData,omitempty"`
	}
	if err := json.Unmarshal(blob, &body); err != nil {
		logging.Warningf(c, "PubSub message is not valid JSON: %s", err)
		return ""
	}
	if body.UserData != nil {
		return string(body.UserData)
	}
	logging.Warningf(c, "crbug.com/1410912: PubSub legacy format")
	return body.UserDataLegacy
}

// HandleNotification is part of Manager interface.
func (m TaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	ctl.DebugLog("Received PubSub notification, asking Buildbucket for the build status")
	return m.checkBuildStatus(c, ctl)
}

// HandleTimer is part of Manager interface.
func (m TaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	if name == statusCheckTimerName {
		if err := m.checkBuildStatus(c, ctl); err != nil {
			// This is either a fatal or transient error. If it is fatal, no need to
			// schedule the timer anymore. If it is transient, HandleTimer call itself
			// will be retried and the timer will be rescheduled then.
			return err
		}
		m.checkBuildStatusLater(c, ctl) // reschedule this check
	}
	return nil
}

// GetDebugState is part of Manager interface.
func (m TaskManager) GetDebugState(c context.Context, ctl task.ControllerReadOnly) (*internal.DebugManagerState, error) {
	return nil, fmt.Errorf("no debug state")
}

func makeServerURL(s string) string {
	if strings.HasPrefix(s, "http://") {
		// Used only in tests where we hardcode http in cfg.Server because local
		// server is http not https.
		return s
	}
	return "https://" + s
}

// withBuildbucket makes a Buildbucket Builds API client and calls the callback.
//
// The callback runs under a new context with 1 min deadline.
func (m TaskManager) withBuildbucket(c context.Context, ctl task.Controller, cb func(context.Context, bbgrpcpb.BuildsClient) error) error {
	c, cancel := clock.WithTimeout(c, time.Minute)
	defer cancel()

	prpcClient := &prpc.Client{Options: prpc.DefaultOptions()}
	var err error
	if prpcClient.C, err = ctl.GetClient(c); err != nil {
		return err
	}

	cfg := ctl.Task().(*messages.BuildbucketTask)
	switch {
	case strings.HasPrefix(cfg.Server, "https://"):
		prpcClient.Host = strings.TrimPrefix(cfg.Server, "https://")
	case strings.HasPrefix(cfg.Server, "http://"):
		prpcClient.Host = strings.TrimPrefix(cfg.Server, "http://")
		prpcClient.Options.Insecure = true
	default:
		prpcClient.Host = cfg.Server
	}

	return cb(c, bbgrpcpb.NewBuildsClient(prpcClient))
}

// checkBuildStatusLater schedules a delayed call to checkBuildStatus if the
// invocation is still running.
//
// This is a fallback mechanism in case PubSub notifications are delayed or
// lost for some reason.
func (m TaskManager) checkBuildStatusLater(c context.Context, ctl task.Controller) {
	if !ctl.State().Status.Final() {
		ctl.AddTimer(c,
			randomDuration(c, statusCheckTimerIntervalMin, statusCheckTimerIntervalMax),
			statusCheckTimerName,
			nil)
	}
}

// randomDuration returns a random seconds duration within the given bounds.
func randomDuration(c context.Context, min, max time.Duration) time.Duration {
	d := min + time.Duration(mathrand.Int63n(c, int64(max-min)))
	return d.Truncate(time.Second)
}

func (m TaskManager) checkBuildStatus(c context.Context, ctl task.Controller) error {
	switch status := ctl.State().Status; {
	// This can happen if Buildbucket manages to send PubSub message before
	// LaunchTask finishes. Do not touch State or DebugLog to avoid collision with
	// still running LaunchTask when saving the invocation, it will only make the
	// matters worse.
	case status == task.StatusStarting:
		return tq.Retry.Apply(transient.Tag.Apply(errors.New("invocation is still starting, try again later")))
	case status != task.StatusRunning:
		return fmt.Errorf("unexpected invocation status %q, expecting %q", status, task.StatusRunning)
	}

	// Grab build ID from the blob generated in LaunchTask.
	taskData, err := readTaskData(ctl)
	if err != nil {
		ctl.State().Status = task.StatusFailed
		return err
	}

	// Fetch the build from Buildbucket.
	var build *bbpb.Build
	err = m.withBuildbucket(c, ctl, func(ctx context.Context, bb bbgrpcpb.BuildsClient) (err error) {
		build, err = bb.GetBuild(ctx, &bbpb.GetBuildRequest{Id: taskData.BuildID})
		return
	})
	if err != nil {
		ctl.DebugLog("Failed to fetch build - %s", err)
		err = grpcutil.WrapIfTransientOr(err, codes.DeadlineExceeded)
		if !transient.Tag.In(err) {
			ctl.State().Status = task.StatusFailed
		}
		return err
	}

	// Switch the invocation status according to the Build status.
	handleBuildStatus(ctl, build)

	// Log the final state of the build or just its status if still running (to be
	// less spammy).
	if ctl.State().Status.Final() {
		ctl.DebugLog("Build:\n%s", protoToJSON(build))
	} else {
		ctl.DebugLog("Build status: %v", build.Status)
	}

	return nil
}

// handleBuildStatus adjusts the invocation state based on the build's status.
func handleBuildStatus(ctl task.Controller, build *bbpb.Build) {
	switch build.Status {
	case bbpb.Status_SCHEDULED, bbpb.Status_STARTED:
		// do nothing, the invocation is still active
	case bbpb.Status_SUCCESS:
		ctl.State().Status = task.StatusSucceeded
	case bbpb.Status_FAILURE, bbpb.Status_INFRA_FAILURE:
		ctl.State().Status = task.StatusFailed
	case bbpb.Status_CANCELED:
		ctl.State().Status = task.StatusAborted
	default:
		ctl.DebugLog("Unexpected Build status %v, marking the invocation as failed", build.Status)
		ctl.State().Status = task.StatusFailed
	}
}

// builderID derives Buildbucket v2 builder ID from the config.
//
// Returns an error if some fields are invalid or there's not enough
// information.
func builderID(cfg *messages.BuildbucketTask, realmID string) (*bbpb.BuilderID, error) {
	var project, bucket string

	switch {
	case cfg.Bucket == "":
		// Fallback to the realm. Ensure it is not a special realm.
		project, bucket = realms.Split(realmID)
		if bucket == realms.LegacyRealm || bucket == realms.RootRealm {
			return nil, fmt.Errorf("'bucket' field for jobs in %q realm is required", bucket)
		}

	case strings.ContainsRune(cfg.Bucket, ':'):
		// Full v2 form "<project>:<bucket>".
		chunks := strings.SplitN(cfg.Bucket, ":", 2)
		project, bucket = chunks[0], chunks[1]

	case strings.HasPrefix(cfg.Bucket, "luci."):
		// Legacy v1 bucket that matches a v2 bucket: "luci.<project>.<bucket>".
		// No longer allowed.
		chunks := strings.SplitN(cfg.Bucket, ".", 3)
		if len(chunks) != 3 {
			return nil, fmt.Errorf("bad legacy v1 'bucket' %q, need 3 components", cfg.Bucket)
		}
		project, bucket = chunks[1], chunks[2]

		var full string
		if curProject, _ := realms.Split(realmID); project != curProject {
			full = fmt.Sprintf("%s:%s", project, bucket)
		} else {
			full = bucket
		}
		return nil, fmt.Errorf("legacy v1 bucket names like %q are no longer allowed, use %q instead", cfg.Bucket, full)

	default:
		// A v2 bucket name within the current project.
		project, _ = realms.Split(realmID)
		bucket = cfg.Bucket
	}

	if cfg.Builder == "" {
		return nil, fmt.Errorf("'builder' field is required")
	}

	return &bbpb.BuilderID{
		Project: project,
		Bucket:  bucket,
		Builder: cfg.Builder,
	}, nil
}

// triggerToCommit converts a gitiles trigger to a buildbucket gitiles commit.
func triggerToCommit(t *scheduler.GitilesTrigger) (*bbpb.GitilesCommit, error) {
	repo, err := gitiles.NormalizeRepoURL(t.Repo, false)
	if err != nil {
		return nil, errors.Fmt("bad repo URL %q: %w", t.Repo, err)
	}
	return &bbpb.GitilesCommit{
		Host:    repo.Host,
		Project: strings.TrimPrefix(repo.Path, "/"),
		Id:      t.Revision,
		Ref:     t.Ref,
	}, nil
}

// popCommitFromTags tries to reconstruct GitilesCommit from tags.
//
// Removes gitiles commit information from properties and tags (modifying them
// in-place), since Buildbucket v2 refuses to accept it there.
//
// See also https://chromium.googlesource.com/infra/infra/+/7a647a9d/appengine/cr-buildbucket/legacy/api.py#101
//
// Returns the extracted commit or nil.
func popCommitFromTags(ctl task.Controller, props *structpb.Struct, tags *[]string) *bbpb.GitilesCommit {
	var commit *bbpb.GitilesCommit
	var ref string

	// Pop all gitiles_ref and buildset tags (usually one of each). They will be
	// reconstructed based on GitilesCommit by Buildbucket.
	kept := (*tags)[:0]
	for _, tag := range *tags {
		switch k, v := strpair.Parse(tag); {
		case k == "gitiles_ref":
			// The last one wins (per BBv1's parse_v1_tags).
			if ref != "" {
				ctl.DebugLog("Ignoring extra gitiles_ref %q", ref)
			}
			ref = normalizeRef(v)

		case k == "buildset":
			// This first one wins (per BBv1's parse_v1_tags).
			if commit != nil {
				ctl.DebugLog("Ignoring extra buildset tag %q", tag)
			} else {
				if commit = parseGitilesBuildset(v); commit != nil {
					ctl.DebugLog("Popped buildset tag %q", tag)
				} else {
					ctl.DebugLog("Ignoring unrecognized buildset tag %q", tag)
				}
			}

		default:
			kept = append(kept, tag)
		}
	}
	*tags = kept

	// Fill in `commit.Ref` based on gitiles_ref tag value.
	if commit != nil {
		commit.Ref = ref
	} else {
		ctl.DebugLog("Ignoring gitiles_ref tag without the buildset tag")
	}

	// Pop reserved properties. BBv2 will reject the request if they are present.
	popProp := func(key string) string {
		if field := props.Fields[key]; field != nil {
			delete(props.Fields, key)
			return field.GetStringValue()
		}
		return ""
	}
	repository := popProp("repository")
	branch := normalizeRef(popProp("branch"))
	revision := popProp("revision") // oddly enough, this property is actually allowed

	// If we had no buildset tag, just discard the properties. They are not
	// authoritative.
	if commit == nil {
		if repository != "" {
			ctl.DebugLog("No buildset tag present, ignoring property %q: %q", "repository", repository)
		}
		if branch != "" {
			ctl.DebugLog("No buildset tag present, ignoring property %q: %q", "branch", branch)
		}
		if revision != "" {
			ctl.DebugLog("No buildset tag present, ignoring property %q: %q", "revision", revision)
		}
		return nil
	}

	// Log if properties disagree with information from tags.
	if repository != "" {
		repoURL, err := gitiles.NormalizeRepoURL(repository, false)
		if err != nil {
			ctl.DebugLog("Ignoring invalid property %q: %q", "repository", repository)
		} else {
			if repoURL.Host != commit.Host {
				ctl.DebugLog("Git host in properties %q doesn't match the one in tags %q", repoURL.Host, commit.Host)
			}
			if proj := strings.TrimPrefix(repoURL.Path, "/"); proj != commit.Project {
				ctl.DebugLog("Git project in properties %q doesn't match the one in tags %q", proj, commit.Project)
			}
		}
	}
	if branch != "" && branch != commit.Ref {
		ctl.DebugLog("Git ref in properties %q doesn't match the one in tags %q", branch, commit.Ref)
	}
	if revision != "" && revision != commit.Id {
		ctl.DebugLog("Git commit in properties %q doesn't match the one in tags %q", revision, commit.Id)
	}

	return commit
}

var gitilesBuildsetRe = regexp.MustCompile(`^commit/gitiles/([^/]+)/(.+?)/\+/([a-f0-9]+)$`)

// parseGitilesBuildset parses Gitiles buildset tag into a proto.
//
// Example input:
//
//	commit/gitiles/chromium.googlesource.com/chromium/src/+/
//	4fa74ef7511f4167d15a5a6d464df06e41ffbd70
//
// Returns nil if `t` doesn't look like a gitiles buildset.
func parseGitilesBuildset(t string) *bbpb.GitilesCommit {
	m := gitilesBuildsetRe.FindStringSubmatch(t)
	if len(m) == 0 {
		return nil
	}
	return &bbpb.GitilesCommit{
		Host:    m[1],
		Project: m[2],
		Id:      m[3],
	}
}

// normalizeRef returns either "refs/..." or "" if `ref` is empty.
func normalizeRef(ref string) string {
	if ref != "" && !strings.HasPrefix(ref, "refs/") {
		ref = "refs/heads/" + ref
	}
	return ref
}

// toBuildbucketPairs converts a list of "key:value" to a list of StringPair.
func toBuildbucketPairs(s []string) []*bbpb.StringPair {
	out := make([]*bbpb.StringPair, len(s))
	for i, kv := range s {
		k, v := strpair.Parse(kv)
		out[i] = &bbpb.StringPair{Key: k, Value: v}
	}
	return out
}

func strProtoValue(s string) *structpb.Value {
	return &structpb.Value{
		Kind: &structpb.Value_StringValue{
			StringValue: s,
		},
	}
}

// protoToJSON is used to pretty-print proto messages in debug logs.
func protoToJSON(p proto.Message) string {
	var buf bytes.Buffer
	if err := (&jsonpb.Marshaler{Indent: "  "}).Marshal(&buf, p); err != nil {
		return fmt.Sprintf("<failed to marshal proto to JSON: %s>", err)
	}
	return buf.String()
}

// schedulerProperty returns "$recipe_engine/scheduler" property value.
//
// The schema of the property is defined in
// https://chromium.googlesource.com/infra/luci/recipes-py/+/HEAD/recipe_modules/scheduler/__init__.py
//
// Note: this function is very inefficient.
func schedulerProperty(ctx context.Context, ctl task.Controller) (*structpb.Value, error) {
	buf := &bytes.Buffer{}

	triggerList := &structpb.ListValue{}
	m := &jsonpb.Marshaler{}
	um := &jsonpb.Unmarshaler{}

	ts := ctl.Request().IncomingTriggers
	if len(ts) > maxTriggersAsSchedulerProperty {
		ctl.DebugLog("Capping %d triggers passed to the build to just %d latest ones",
			len(ts), maxTriggersAsSchedulerProperty)
		ts = ts[len(ts)-maxTriggersAsSchedulerProperty:]
	}
	for _, tInternal := range ts {
		buf.Reset()
		tPublic := internal.ToPublicTrigger(tInternal)
		if err := m.Marshal(buf, tPublic); err != nil {
			return nil, err
		}
		tStruct := &structpb.Struct{}
		if err := um.Unmarshal(buf, tStruct); err != nil {
			return nil, err
		}
		triggerList.Values = append(triggerList.Values, &structpb.Value{
			Kind: &structpb.Value_StructValue{StructValue: tStruct},
		})
	}

	return &structpb.Value{
		Kind: &structpb.Value_StructValue{
			StructValue: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"hostname": {
						Kind: &structpb.Value_StringValue{
							StringValue: info.DefaultVersionHostname(ctx),
						},
					},
					"job": {
						Kind: &structpb.Value_StringValue{
							StringValue: ctl.JobID(),
						},
					},
					"invocation": {
						Kind: &structpb.Value_StringValue{
							StringValue: fmt.Sprintf("%d", ctl.InvocationID()),
						},
					},
					"triggers": {
						Kind: &structpb.Value_ListValue{ListValue: triggerList},
					},
				},
			},
		},
	}, nil
}

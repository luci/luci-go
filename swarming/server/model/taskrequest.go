// Copyright 2023 The LUCI Authors.
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

package model

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	configpb "go.chromium.org/luci/swarming/proto/config"
)

// taskRequestIDMask is xored with TaskRequest entity ID.
const taskRequestIDMask = 0x7fffffffffffffff

// TaskRequest contains a user request to execute a task.
//
// Key ID is a decreasing integer based on time plus some randomness on lower
// order bits. See NewTaskRequestID for the complete gory details.
//
// This entity is immutable.
type TaskRequest struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key is derived based on time and randomness.
	//
	// It is normally serialized into a hex string. See TaskRequestKey.
	Key *datastore.Key `gae:"$key"`

	// TxnUUID is used internally to make the transaction that creates TaskRequest
	// idempotent.
	//
	// Just a randomly generated string. Should not be used for anything else.
	// Should not show up anywhere.
	TxnUUID string `gae:"txn_uuid,noindex"`

	// TaskSlices defines what to run.
	//
	// Each slice defines what to run and where. Slices are attempted one after
	// another, until some ends up running on a bot. If an attempt to schedule
	// a particular slice fails (i.e. there are no bots matching requested
	// dimensions or the slice sits queued for too long and expires), the next
	// slice is scheduled in its place.
	//
	// This is primarily used to requests bots with "hot" caches before falling
	// back on more generic bots.
	TaskSlices []TaskSlice `gae:"task_slices,lsp,noindex"`

	// LegacyProperties is no longer used and should be null.
	LegacyProperties LegacyNullProperty `gae:"properties"`

	// CreatedTS is a timestamp when this request was registered.
	CreatedTS time.Time `gae:"created_ts"`

	// ExpirationTS is when to give up trying to run the task.
	//
	// If the task request is not scheduled by this moment, it will be aborted
	// with EXPIRED status. This value always matches ExpirationTS of the last
	// TaskSlice.
	//
	// TODO(vadimsh): Why is it stored separately at all?
	ExpirationTS time.Time `gae:"expiration_ts,noindex"`

	// Name of this task request as provided by the caller. Only for description.
	//
	// TODO(vadimsh): Why is it indexed?
	Name string `gae:"name"`

	// ParentTaskID is set when this task was created from another task.
	//
	// This is packed TaskToRun ID of an attempt that launched this task or an
	// empty string if this task doesn't have a parent.
	ParentTaskID string `gae:"parent_task_id"`

	// Authenticated is an identity that triggered this task.
	//
	// Derived from the caller credentials.
	Authenticated identity.Identity `gae:"authenticated"`

	// What user to "blame" for this task.
	//
	// Can be arbitrary, not asserted by any credentials.
	User string `gae:"user"`

	// Tags classify this task in some way.
	//
	// This is a generated property. This property contains both the tags
	// specified by the user and the tags from every TaskSlice.
	Tags []string `gae:"tags"`

	// ManualTags are tags that are provided by the user.
	//
	// This is used to regenerate the list of tags for TaskResultSummary based on
	// the actual TaskSlice used.
	ManualTags []string `gae:"manual_tags,noindex"`

	// ServiceAccount indicates what credentials the task uses when calling other
	// services.
	//
	// Possible values are: `none`, `bot` or `<email>`.
	ServiceAccount string `gae:"service_account"`

	// Realm is task's realm controlling who can see and cancel this task.
	Realm string `gae:"realm"`

	// RealmsEnabled is a legacy flag that should always be True.
	//
	// TODO(vadimsh): Get rid of it when Python code is no more.
	RealmsEnabled bool `gae:"realms_enabled,noindex"`

	// SchedulingAlgorithm is a scheduling algorithm set in pools.cfg at the time
	// the request was created.
	//
	// TODO(vadimsh): This does nothing for RBE pools.
	SchedulingAlgorithm configpb.Pool_SchedulingAlgorithm `gae:"scheduling_algorithm,noindex"`

	// Priority of the this task.
	//
	// A lower number means higher priority.
	Priority int64 `gae:"priority,noindex"`

	// BotPingToleranceSecs is a maximum delay between bot pings before the bot is
	// considered dead while running a task.
	//
	// TODO(vadimsh): Why do this per-task instead of per-pool or even hardcoded?
	BotPingToleranceSecs int64 `gae:"bot_ping_tolerance_secs,noindex"`

	// RBEInstance is an RBE instance to send the task to or "" to use Swarming
	// native scheduler.
	//
	// Initialized when creating a task based on pools.cfg config.
	RBEInstance string `gae:"rbe_instance,noindex"`

	// PubSubTopic is a topic to send a task completion notification to.
	PubSubTopic string `gae:"pubsub_topic,noindex"`

	// PubSubAuthToken is a secret token to send as `auth_token` PubSub message
	// attribute.
	PubSubAuthToken string `gae:"pubsub_auth_token,noindex"`

	// PubSubUserData is data to send in `userdata` field of PubSub messages.
	PubSubUserData string `gae:"pubsub_userdata,noindex"`

	// ResultDBUpdateToken is the ResultDB invocation's update token for the task
	// run that was created for this request.
	//
	// This is empty if the task was deduplicated or if ResultDB integration was
	// not enabled for this task.
	ResultDBUpdateToken string `gae:"resultdb_update_token,noindex"`

	// ResultDB is ResultDB integration configuration for this task.
	ResultDB ResultDBConfig `gae:"resultdb,lsp,noindex"`

	// HasBuildTask is true if the TaskRequest has an associated BuildTask.
	HasBuildTask bool `gae:"has_build_task,noindex"`
}

// TaskSlice defines where and how to run the task and when to give up.
//
// The task will fallback from one slice to the next until it finds a matching
// bot.
//
// This entity is not saved in the DB as a standalone entity, instead it is
// embedded in a TaskRequest.
//
// This entity is immutable.
type TaskSlice struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Properties defines where and how to run the task.
	//
	// If a task is marked as an idempotent (see TaskProperties), property values
	// are hashed in a reproducible way and the final hash is used to find a
	// previously succeeded task that has the same properties.
	Properties TaskProperties `gae:"properties,lsp,noindex"`

	// ExpirationSecs defines how long the slice can sit in a pending queue.
	//
	// If this task slice is not scheduled by this moment, the next one will be
	// enqueued instead.
	ExpirationSecs int64 `gae:"expiration_secs"`

	// WaitForCapacity is a legacy flag that does nothing, always false now.
	//
	// TODO(vadimsh): Remove it when no longer referenced
	WaitForCapacity bool `gae:"wait_for_capacity"`
}

// TaskProperties defines where and how to run the task.
//
// This entity is not saved in the DB as a standalone entity, instead it is
// embedded in a TaskSlice.
//
// This entity is immutable.
type TaskProperties struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Idempotent, if true, means it is OK to skip running this task if there's
	// already a successful task with the same properties hash.
	//
	// The results of such previous task will be reused as results of this task.
	Idempotent bool `gae:"idempotent"`

	// Dimensions are used to match this task to a bot.
	//
	// This is conceptually a set of `(key, value1 | value2 | ...)` pairs, each
	// defining some constraint on a matching bot. For a bot to match the task,
	// it should satisfy all constraints.
	//
	// For a bot to match a single `(key, value1 | value2| ...)` constraint, bot's
	// value for dimension `key` should be equal to `value1` or `value2` and so
	// on.
	Dimensions TaskDimensions `gae:"dimensions"`

	// ExecutionTimeoutSecs is the maximum duration the bot can take to run this
	// task.
	//
	// It's also known as `hard_timeout` in the bot code.
	ExecutionTimeoutSecs int64 `gae:"execution_timeout_secs"`

	// GracePeriodSecs is the time between sending SIGTERM and SIGKILL when the
	// task times out.
	//
	// As soon as the ask reaches its execution timeout, the task process is sent
	// SIGTERM. The process should clean up and terminate. If it is still running
	// after GracePeriodSecs, it gets killed via SIGKILL.
	GracePeriodSecs int64 `gae:"grace_period_secs"`

	// IOTimeoutSecs controls how soon to consider a "silent" process to be stuck.
	//
	// If a subprocess doesn't output new data to stdout for  IOTimeoutSecs,
	// consider the task timed out. Optional.
	IOTimeoutSecs int64 `gae:"io_timeout_secs"`

	// Command is a command line to run.
	Command []string `gae:"command"`

	// RelativeCwd is a working directory relative to the task root to run
	// the command in.
	RelativeCwd string `gae:"relative_cwd"`

	// Env is environment variables to set when running the task process.
	Env Env `gae:"env"`

	// EnvPrefixes is environment path prefix variables.
	//
	// E.g. if a `PATH` key has values `[a, b]`, then the final `PATH` env var
	// will be `a;b;$PATH` (where `;` is a platforms' env path separator).
	EnvPrefixes EnvPrefixes `gae:"env_prefixes"`

	// Caches defines what named caches to mount.
	Caches []CacheEntry `gae:"caches,lsp"`

	// CASInputRoot is a digest of the input root uploaded to RBE-CAS.
	//
	// This MUST be digest of `build.bazel.remote.execution.v2.Directory`.
	CASInputRoot CASReference `gae:"cas_input_root,lsp"`

	// CIPDInput defines what CIPD packages to install.
	CIPDInput CIPDInput `gae:"cipd_input,lsp"`

	// LegacyInputsRef is no longer used and should be null.
	LegacyInputsRef LegacyNullProperty `gae:"inputs_ref"`

	// Outputs is a list of extra outputs to upload to RBE-CAS as task results.
	//
	// If empty, only files written to `${ISOLATED_OUTDIR}` will be returned.
	// Otherwise, the files in this list will be added to those in that directory.
	Outputs []string `gae:"outputs"`

	// HasSecretBytes, if true, means there's a SecretBytes entity associated with
	// the parent TaskRequest.
	HasSecretBytes bool `gae:"has_secret_bytes"`

	// Containment defines what task process containment mechanism to use.
	//
	// Not really implemented currently.
	Containment Containment `gae:"containment,lsp"`
}

// CacheEntry describes a named cache that should be present on the bot.
type CacheEntry struct {
	// Name is a logical cache name.
	Name string `gae:"name"`
	// Path is where to mount it relative to the task root directory.
	Path string `gae:"path"`
}

// CASReference described where to fetch input files from.
type CASReference struct {
	// CASInstance is a full name of RBE-CAS instance.
	CASInstance string `gae:"cas_instance"`
	// Digest identifies the root tree to fetch.
	Digest CASDigest `gae:"digest,lsp"`
}

// CASDigest represents an RBE-CAS blob's digest.
//
// Is is a representation of build.bazel.remote.execution.v2.Digest. See
// https://github.com/bazelbuild/remote-apis/blob/77cfb44a88577a7ade5dd2400425f6d50469ec6d/build/bazel/remote/execution/v2/remote_execution.proto#L753-L791
type CASDigest struct {
	// Hash is blob's hash digest as a hex string.
	Hash string `gae:"hash"`
	// SizeBytes is the blob size.
	SizeBytes int64 `gae:"size_bytes"`
}

// CIPDInput specifies which CIPD client and packages to install.
type CIPDInput struct {
	// Server is URL of the CIPD server (including "https://" schema).
	Server string `gae:"server"`
	// ClientPackage defines a version of the CIPD client to use.
	ClientPackage CIPDPackage `gae:"client_package,lsp"`
	// Packages is a list of packages to install.
	Packages []CIPDPackage `gae:"packages,lsp"`
}

// CIPDPackage defines a CIPD package to install into the task directory.
type CIPDPackage struct {
	// PackageName is a package name template (e.g. may include `${platform}`).
	PackageName string `gae:"package_name"`
	// Version is a package version to install.
	Version string `gae:"version"`
	// Path is a path relative to the task directory where to install the package.
	Path string `gae:"path"`
}

// Containment describes the task process containment.
type Containment struct {
	LowerPriority             bool  `gae:"lower_priority"`
	ContainmentType           int64 `gae:"containment_type"`
	LimitProcesses            int64 `gae:"limit_processes"`
	LimitTotalCommittedMemory int64 `gae:"limit_total_committed_memory"`
}

// ResultDBConfig is ResultDB integration configuration for a task.
type ResultDBConfig struct {
	// Enable indicates if the task should have ResultDB invocation.
	//
	// If True and this task is not deduplicated, create
	// "task-{swarming_hostname}-{run_id}" invocation for this task, provide its
	// update token to the task subprocess via LUCI_CONTEXT and finalize the
	// invocation when the task is done.
	//
	// If the task is deduplicated, then TaskResult.InvocationName will be the
	// invocation name of the original task.
	Enable bool `gae:"enable"`
}

// TaskDimensions defines requirements for a bot to match a task.
//
// Stored in JSON form in the datastore.
type TaskDimensions map[string][]string

// ToProperty stores the value as a JSON-blob property.
func (p *TaskDimensions) ToProperty() (datastore.Property, error) {
	return ToJSONProperty(p)
}

// FromProperty loads a JSON-blob property.
func (p *TaskDimensions) FromProperty(prop datastore.Property) error {
	return FromJSONProperty(prop, p)
}

// Env is a list of `(key, value)` pairs with environment variables to set.
//
// Stored in JSON form in the datastore.
type Env map[string]string

// ToProperty stores the value as a JSON-blob property.
func (p *Env) ToProperty() (datastore.Property, error) {
	return ToJSONProperty(p)
}

// FromProperty loads a JSON-blob property.
func (p *Env) FromProperty(prop datastore.Property) error {
	return FromJSONProperty(prop, p)
}

// EnvPrefixes is a list of `(key, []value)` pairs with env prefixes to add.
//
// Stored in JSON form in the datastore.
type EnvPrefixes map[string][]string

// ToProperty stores the value as a JSON-blob property.
func (p *EnvPrefixes) ToProperty() (datastore.Property, error) {
	return ToJSONProperty(p)
}

// FromProperty loads a JSON-blob property.
func (p *EnvPrefixes) FromProperty(prop datastore.Property) error {
	return FromJSONProperty(prop, p)
}

////////////////////////////////////////////////////////////////////////////////

// TaskRequestKey returns TaskRequest entity key given a task ID string.
//
// The task ID is something that looks like "60b2ed0a43023110", it is either
// a "packed TaskResultSummary key" (when ends with 0) or "a packed
// TaskRunResult key" (when ends with non-0).
//
// Task request key is a root key of the hierarchy of entities representing
// a particular task. All key constructor functions for such entities take
// the request key as an argument.
func TaskRequestKey(ctx context.Context, taskID string) (*datastore.Key, error) {
	if err := checkIsHex(taskID, 2); err != nil {
		return nil, errors.Annotate(err, "bad task ID").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	// Chop the suffix byte. It is TaskRunResult index, we don't care about it.
	num, err := strconv.ParseInt(taskID[:len(taskID)-1], 16, 64)
	if err != nil {
		return nil, errors.Annotate(err, "bad task ID").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	return datastore.NewKey(ctx, "TaskRequest", "", num^taskRequestIDMask, nil), nil
}

// NewTaskRequestID generates an ID for a new task.
func NewTaskRequestID(ctx context.Context) int64 {
	panic("not implemented")
}

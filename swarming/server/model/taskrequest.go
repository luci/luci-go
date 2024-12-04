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
	"crypto/md5"
	"encoding/binary"
	"reflect"
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
)

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

	// Created is a precise timestamp when this request was registered.
	//
	// Unlike the timestamp in the entity key, this one has microsecond precision.
	//
	// The index is used in BQ exports and when cleaning up old tasks.
	//
	// TODO(vadimsh): BQ export can be switched to use __key__ and old tasks
	// should be cleaned up via a TTL policy. Then we can remove this index.
	Created time.Time `gae:"created_ts"`

	// Expiration is when to give up trying to run the task.
	//
	// If the task request is not scheduled by this moment, it will be aborted
	// with EXPIRED status. This value always matches Expiration of the last
	// TaskSlice.
	//
	// TODO(vadimsh): Why is it stored separately at all?
	Expiration time.Time `gae:"expiration_ts,noindex"`

	// Name of this task request as provided by the caller. Only for description.
	Name string `gae:"name,noindex"`

	// ParentTaskID is set when this task was created from another task.
	//
	// This is packed TaskToRun ID of an attempt that launched this task or null
	// if this task doesn't have a parent.
	//
	// The index is used to find children of a particular parent task to cancel
	// them when the parent task dies.
	ParentTaskID datastore.Nullable[string, datastore.Indexed] `gae:"parent_task_id"`

	// RootTaskID identifies the task run that started the tree of Swarming tasks.
	//
	// If a new task doesn't have a parent, this is unset. Otherwise if the parent
	// task has RootTaskID, this value is used in the new task. Otherwise
	// ParentTaskID itself is used.
	//
	// That way all tasks from the same task tree (except the root one itself)
	// will have RootTaskID populated.
	//
	// This is used in BQ exported. Not clear if anyone actually consumes this
	// information.
	RootTaskID string `gae:"root_task_id,noindex"`

	// Authenticated is an identity that triggered this task.
	//
	// Derived from the caller credentials.
	Authenticated identity.Identity `gae:"authenticated,noindex"`

	// What user to "blame" for this task.
	//
	// Can be arbitrary, not asserted by any credentials.
	User string `gae:"user,noindex"`

	// Tags classify this task in some way.
	//
	// This is a generated property. This property contains both the tags
	// specified by the user and the tags from every TaskSlice.
	Tags []string `gae:"tags,noindex"`

	// ManualTags are tags that are provided by the user.
	//
	// This is used to regenerate the list of tags for TaskResultSummary based on
	// the actual TaskSlice used.
	ManualTags []string `gae:"manual_tags,noindex"`

	// ServiceAccount indicates what credentials the task uses when calling other
	// services.
	//
	// Possible values are: `none`, `bot` or `<email>`.
	ServiceAccount string `gae:"service_account,noindex"`

	// Realm is task's realm controlling who can see and cancel this task.
	//
	// Missing for internally generated tasks such as termination tasks.
	Realm string `gae:"realm,noindex"`

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

	// LegacyProperties is no longer used.
	LegacyProperties LegacyProperty `gae:"properties"`

	// LegacyHasBuildToken is no longer used.
	LegacyHasBuildToken LegacyProperty `gae:"has_build_token"`
}

// Pool is the pool the task wants to run in.
func (p *TaskRequest) Pool() string {
	pool, ok := p.TaskSlices[0].Properties.Dimensions["pool"]
	if !ok || len(pool) == 0 {
		return ""
	}
	return pool[0]
}

// BotID is a specific bot the task wants to run on, if any.
func (p *TaskRequest) BotID() string {
	botID, ok := p.TaskSlices[0].Properties.Dimensions["id"]
	if !ok || len(botID) == 0 {
		return ""
	}
	return botID[0]
}

// TaskAuthInfo returns information about the task for ACL checks.
//
// This implements acls.Task.
func (p *TaskRequest) TaskAuthInfo(ctx context.Context) (*acls.TaskAuthInfo, error) {
	return &acls.TaskAuthInfo{
		TaskID:    RequestKeyToTaskID(p.Key, AsRequest),
		Realm:     p.Realm,
		Pool:      p.Pool(),
		BotID:     p.BotID(),
		Submitter: p.Authenticated,
	}, nil
}

// ToProto converts a TaskRequest to apipb.TaskRequestResponse.
func (p *TaskRequest) ToProto() *apipb.TaskRequestResponse {
	taskSlices := make([]*apipb.TaskSlice, len(p.TaskSlices))
	for i, slice := range p.TaskSlices {
		taskSlices[i] = slice.ToProto()
	}
	properties := &apipb.TaskProperties{}
	if len(taskSlices) != 0 {
		properties = taskSlices[0].Properties
	}
	taskID := ""
	if p.Key != nil {
		taskID = RequestKeyToTaskID(p.Key, AsRequest)
	}
	return &apipb.TaskRequestResponse{
		TaskId:               taskID,
		ExpirationSecs:       int32(p.Expiration.Sub(p.Created) / time.Second),
		Name:                 p.Name,
		ParentTaskId:         p.ParentTaskID.Get(),
		Priority:             int32(p.Priority),
		Properties:           properties,
		Tags:                 p.Tags,
		CreatedTs:            timestamppb.New(p.Created),
		User:                 p.User,
		Authenticated:        string(p.Authenticated),
		TaskSlices:           taskSlices,
		ServiceAccount:       p.ServiceAccount,
		Realm:                p.Realm,
		PubsubTopic:          p.PubSubTopic,
		PubsubUserdata:       p.PubSubUserData,
		BotPingToleranceSecs: int32(p.BotPingToleranceSecs),
		RbeInstance:          p.RBEInstance,
		Resultdb:             p.ResultDB.ToProto(),
	}
}

// TaskSlice defines where and how to run the task and when to give up.
//
// The task will fallback from one slice to the next until it finds a matching
// bot.
//
// This entity is not saved in the DB as a standalone entity, instead it is
// embedded in a TaskRequest, unindexed.
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
	Properties TaskProperties `gae:"properties,lsp"`

	// PropertiesHash is a precalculated hash of properties.
	//
	// Populated when the task is scheduled. It is used for task deduplication and
	// in BQ exports.
	PropertiesHash []byte `gae:"properties_hash,noindex"`

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

// ToProto returns an apipb.TaskSlice version of the TaskSlice.
func (p *TaskSlice) ToProto() *apipb.TaskSlice {
	ts := &apipb.TaskSlice{
		ExpirationSecs:  int32(p.ExpirationSecs),
		WaitForCapacity: p.WaitForCapacity,
	}
	if properties := p.Properties.ToProto(); properties != nil {
		ts.Properties = properties
	}
	return ts
}

// TaskProperties defines where and how to run the task.
//
// This entity is not saved in the DB as a standalone entity, instead it is
// embedded in a TaskSlice, unindexed.
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

	// LegacyInputsRef is no longer used.
	LegacyInputsRef LegacyProperty `gae:"inputs_ref"`
}

// ToProto converts TaskProperties to apipb.TaskProperties.
func (p *TaskProperties) ToProto() *apipb.TaskProperties {
	if reflect.DeepEqual(*p, TaskProperties{}) {
		return nil
	}
	caches := make([]*apipb.CacheEntry, len(p.Caches))
	for i, cache := range p.Caches {
		caches[i] = cache.ToProto()
	}
	taskProperties := &apipb.TaskProperties{
		Caches:               caches,
		CipdInput:            p.CIPDInput.ToProto(),
		Command:              p.Command,
		RelativeCwd:          p.RelativeCwd,
		Dimensions:           p.Dimensions.ToProto(),
		Env:                  p.Env.ToProto(),
		EnvPrefixes:          p.EnvPrefixes.ToProto(),
		ExecutionTimeoutSecs: int32(p.ExecutionTimeoutSecs),
		GracePeriodSecs:      int32(p.GracePeriodSecs),
		Idempotent:           p.Idempotent,
		CasInputRoot:         p.CASInputRoot.ToProto(),
		IoTimeoutSecs:        int32(p.IOTimeoutSecs),
		Outputs:              p.Outputs,
		Containment:          p.Containment.ToProto(),
	}
	if p.HasSecretBytes {
		taskProperties.SecretBytes = []byte("<REDACTED>")
	}
	return taskProperties
}

// IsTerminate checks if the request is for a termination task.
//
// Properties of a termination task should only have an id dimension.
func (p *TaskProperties) IsTerminate() bool {
	terminateLikeProps := TaskProperties{
		Dimensions: p.Dimensions,
	}
	if !reflect.DeepEqual(*p, terminateLikeProps) {
		return false
	}

	if len(p.Dimensions) > 1 {
		return false
	}

	_, ok := p.Dimensions["id"]
	return ok
}

// CacheEntry describes a named cache that should be present on the bot.
type CacheEntry struct {
	// Name is a logical cache name.
	Name string `gae:"name"`
	// Path is where to mount it relative to the task root directory.
	Path string `gae:"path"`
}

// ToProto converts CacheEntry to apipb.CacheEntry.
func (p *CacheEntry) ToProto() *apipb.CacheEntry {
	if p.Name == "" && p.Path == "" {
		return nil
	}
	return &apipb.CacheEntry{
		Name: p.Name,
		Path: p.Path,
	}
}

// CASReference described where to fetch input files from.
type CASReference struct {
	// CASInstance is a full name of RBE-CAS instance.
	CASInstance string `gae:"cas_instance"`
	// Digest identifies the root tree to fetch.
	Digest CASDigest `gae:"digest,lsp"`
}

// ToProto converts CASReference to apipb.CASReference.
func (p *CASReference) ToProto() *apipb.CASReference {
	if p.CASInstance == "" && p.Digest.Hash == "" && p.Digest.SizeBytes == 0 {
		return nil
	}
	return &apipb.CASReference{
		CasInstance: p.CASInstance,
		Digest:      p.Digest.ToProto(),
	}
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

// ToProto converts CASDigest to apipb.CASDigest.
func (p *CASDigest) ToProto() *apipb.Digest {
	if p.Hash == "" {
		return nil
	}
	return &apipb.Digest{
		Hash:      p.Hash,
		SizeBytes: p.SizeBytes,
	}
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

// IsPopulated returns true if the struct carries some data.
func (p *CIPDInput) IsPopulated() bool {
	return p.Server != "" || p.ClientPackage.PackageName != "" || len(p.Packages) != 0
}

// ToProto converts CIPDInput to apipb.CIPDInput.
func (p *CIPDInput) ToProto() *apipb.CipdInput {
	if !p.IsPopulated() {
		return nil
	}
	packages := make([]*apipb.CipdPackage, len(p.Packages))
	for i, pkg := range p.Packages {
		packages[i] = pkg.ToProto()
	}
	return &apipb.CipdInput{
		Server:        p.Server,
		ClientPackage: p.ClientPackage.ToProto(),
		Packages:      packages,
	}
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

// ToProto converts CIPDPackage to apipb.CipdPackage.
func (p *CIPDPackage) ToProto() *apipb.CipdPackage {
	if p.PackageName == "" {
		return nil
	}
	return &apipb.CipdPackage{
		PackageName: p.PackageName,
		Version:     p.Version,
		Path:        p.Path,
	}
}

// Containment describes the task process containment.
type Containment struct {
	ContainmentType           apipb.Containment_ContainmentType `gae:"containment_type"`
	LowerPriority             bool                              `gae:"lower_priority"`
	LimitProcesses            int64                             `gae:"limit_processes"`
	LimitTotalCommittedMemory int64                             `gae:"limit_total_committed_memory"`
}

// ToProto converts Containment struct to apipb.Containment
func (p *Containment) ToProto() *apipb.Containment {
	if p.ContainmentType == 0 {
		return nil
	}
	return &apipb.Containment{
		ContainmentType: p.ContainmentType,
	}
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

// ToProto converts ResultDBConfig struct to apipb.ResultDBCfg.
func (p *ResultDBConfig) ToProto() *apipb.ResultDBCfg {
	return &apipb.ResultDBCfg{
		Enable: p.Enable,
	}
}

// SecretBytes defines an optional secret byte string logically defined within
// TaskRequest.
//
// Stored separately for size and data-leakage reasons. All task slices reuse
// the same secret bytes (which is an implementation artifact, not a desired
// property). If a task slice uses secret bytes, it has HasSecretBytes == true.
type SecretBytes struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key identifies the task and its concrete slice.
	//
	// See SecretBytesKey.
	Key *datastore.Key `gae:"$key"`

	// SecretBytes is the actual secret bytes blob.
	SecretBytes []byte `gae:"secret_bytes,noindex"`
}

// SecretBytesKey constructs SecretBytes key given a TaskRequest key.
func SecretBytesKey(ctx context.Context, taskReq *datastore.Key) *datastore.Key {
	return datastore.NewKey(ctx, "SecretBytes", "", 1, taskReq)
}

// TaskRequestID defines a mapping between request's idempotency ID and task ID.
//
// It is a root-level entity. Used to make sure at most one TaskRequest entity
// is created per the given request ID.
type TaskRequestID struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key is derived from the request ID.
	//
	// See TaskRequestIDKey.
	Key *datastore.Key `gae:"$key"`

	// TaskID is a packed TaskResultSummary key identifying TaskRequest matching
	// this request ID.
	//
	// Use TaskRequestKey(...) to get the actual datastore key from it.
	TaskID string `gae:"task_id,noindex"`

	// ExpireAt is when this entity should be removed from the datastore.
	//
	// This is used by a TTL policy: https://cloud.google.com/datastore/docs/ttl
	ExpireAt time.Time `gae:"expire_at,noindex"`
}

// TaskRequestIDKey constructs a top-level TaskRequestID key.
func TaskRequestIDKey(ctx context.Context, requestID string) *datastore.Key {
	return datastore.NewKey(ctx, "TaskRequestID", requestID, 0, nil)
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

// ToProto converts TaskDimensions to []*apipb.StringPair
func (p TaskDimensions) ToProto() []*apipb.StringPair {
	if len(p) == 0 {
		return nil
	}
	td := []*apipb.StringPair{}
	for k, v := range p {
		for _, val := range v {
			td = append(td, &apipb.StringPair{
				Key:   k,
				Value: val,
			})
		}
	}
	SortStringPairs(td)
	return td
}

// Hash returns a 32 bits unsigned int that is a hash of TaskDimensions.
//
// Dimensions values still have "|" in them, i.e. this is calculated prior to
// expansion of OR-ed dimensions into a disjunction of dimension sets.
//
// Dimensions values are expected to be sorted, and it's also expected the
// elements in OR-ed dimensions to be sorted.
//
// The return value is guaranteed to be a non-zero int so it can be used as
// an entity ID.
func (p TaskDimensions) Hash() uint32 {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	delimiter := []byte{0}
	hash := md5.New()
	for _, k := range keys {
		hash.Write([]byte(k))
		hash.Write(delimiter)
		for _, v := range p[k] {
			hash.Write([]byte(v))
			hash.Write(delimiter)
		}
	}
	sum := hash.Sum(nil)
	res := binary.LittleEndian.Uint32(sum[:4])
	if res == 0 {
		return uint32(1)
	}
	return res
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

// ToProto converts Env to []*apipb.StringPair
func (p *Env) ToProto() []*apipb.StringPair {
	if len(*p) == 0 {
		return nil
	}
	sp := []*apipb.StringPair{}
	for k, v := range *p {
		sp = append(sp, &apipb.StringPair{
			Key:   k,
			Value: v,
		})
	}
	SortStringPairs(sp)
	return sp
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

// ToProto converts EnvPrefixes to []*apipb.StringListPair
func (p EnvPrefixes) ToProto() []*apipb.StringListPair {
	return MapToStringListPair((map[string][]string)(p), true)
}

// NewTaskRequestID generates an ID for a new task.
func NewTaskRequestID(ctx context.Context) int64 {
	panic("not implemented")
}

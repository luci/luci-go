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
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/klauspost/compress/zlib"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/packedintset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
)

const (
	// ChunkSize is the size of each TaskOutputChunk.Chunk in uncompressed form.
	//
	// The rationale is that appending data to an entity requires reading it
	// first, so it must not be too big. On the other hand, having thousands of
	// small entities is pure overhead.
	//
	// Note that changing this value is a breaking change for existing logs.
	ChunkSize = 100 * 1024

	// MaxChunks sets a limit on how much of a task output to store.
	MaxChunks = 1024
)

// TaskResultCommon contains properties that are common to both TaskRunResult
// and TaskResultSummary.
//
// It is not meant to be stored in the datastore on its own, only as an embedded
// struct inside TaskRunResult or TaskResultSummary.
type TaskResultCommon struct {
	// State is the current state of the task.
	//
	// The index is used in task listing queries to filter by.
	State apipb.TaskState `gae:"state"`

	// Modified is when this entity was created or last modified.
	Modified time.Time `gae:"modified_ts,noindex"`

	// BotVersion is a version of the bot code running the task, if already known.
	BotVersion string `gae:"bot_version,noindex"`

	// BotDimensions are bot dimensions at the moment the bot claimed the task.
	BotDimensions BotDimensions `gae:"bot_dimensions"`

	// BotIdleSince is when the bot running this task finished its previous task,
	// if already known.
	BotIdleSince datastore.Optional[time.Time, datastore.Unindexed] `gae:"bot_idle_since_ts"`

	// BotLogsCloudProject is a GCP project where the bot uploads its logs.
	BotLogsCloudProject string `gae:"bot_logs_cloud_project,noindex"`

	// ServerVersions is a set of server version(s) that touched this entity.
	ServerVersions []string `gae:"server_versions,noindex"`

	// CurrentTaskSlice is an index of the active task slice in the TaskRequest.
	//
	// For TaskResultSummary it is the currently running slice (and it changes
	// over lifetime of a task if it has many slices). For TaskRunResult it is the
	// index representing this concrete run.
	CurrentTaskSlice int64 `gae:"current_task_slice,noindex"`

	// Started is when the bot started executing the task.
	//
	// The index is used in task listing queries to order by this property.
	Started datastore.Nullable[time.Time, datastore.Indexed] `gae:"started_ts"`

	// Completed is when the task switched into a final state.
	//
	// This is either when the bot finished executing it, or when it expired or
	// was canceled.
	//
	// The index is used in a bunch of places:
	//   1. In task listing queries to order by this property.
	//   2. In crashed bot detection to list still pending or running tasks.
	//   3. In BQ export pagination.
	Completed datastore.Nullable[time.Time, datastore.Indexed] `gae:"completed_ts"`

	// Abandoned is set when a task had an internal failure, timed out or was
	// killed by a client request.
	//
	// The index is used in task listing queries to order by this property.
	Abandoned datastore.Nullable[time.Time, datastore.Indexed] `gae:"abandoned_ts"`

	// DurationSecs is how long the task process was running.
	//
	// This is reported explicitly by the bot and it excludes all overhead.
	DurationSecs datastore.Optional[float64, datastore.Unindexed] `gae:"durations"` // legacy plural property name

	// ExitCode is the task process exit code for tasks in COMPLETED state.
	ExitCode datastore.Optional[int64, datastore.Unindexed] `gae:"exit_codes"` // legacy plural property name

	// Failure is true if the task finished with a non-zero process exit code.
	//
	// The index is used in task listing queries to filter by failure.
	Failure bool `gae:"failure"`

	// InternalFailure is true if the task failed due to an internal error.
	InternalFailure bool `gae:"internal_failure,noindex"`

	// StdoutChunks is the number of TaskOutputChunk entities with the output.
	StdoutChunks int64 `gae:"stdout_chunks,noindex"`

	// CASOutputRoot is the digest of the output root uploaded to RBE-CAS.
	CASOutputRoot CASReference `gae:"cas_output_root,lsp,noindex"`

	// CIPDPins is resolved versions of all the CIPD packages used in the task.
	CIPDPins CIPDInput `gae:"cipd_pins,lsp,noindex"`

	// MissingCIPD is the missing CIPD packages in CLIENT_ERROR state.
	MissingCIPD []CIPDPackage `gae:"missing_cipd,lsp,noindex"`

	// MissingCAS is the missing CAS digests in CLIENT_ERROR state.
	MissingCAS []CASReference `gae:"missing_cas,lsp,noindex"`

	// ResultDBInfo is ResultDB invocation info for this task.
	//
	// If this task was deduplicated, this contains invocation info from the
	// previously ran the task deduplicated from.
	ResultDBInfo ResultDBInfo `gae:"resultdb_info,lsp,noindex"`

	// LegacyOutputsRef is no longer used.
	LegacyOutputsRef LegacyProperty `gae:"outputs_ref"`
}

// toProto populates as much of apipb.TaskResultResponse as it can.
//
// It is used as part of TaskRunResult.ToProto and TaskResultSummary.ToProto.
//
// Fields that still need to be populated by the caller:
//   - BotId
//   - CostSavedUsd
//   - CostsUsd
//   - CreatedTs
//   - DedupedFrom
//   - Name
//   - PerformanceStats
//   - RunId
//   - Tags
//   - TaskId
//   - User
func (p *TaskResultCommon) toProto() *apipb.TaskResultResponse {
	type timestamp interface {
		IsSet() bool
		Get() time.Time
	}
	timeToTimestampPB := func(t timestamp) *timestamppb.Timestamp {
		if !t.IsSet() {
			return nil
		}
		return timestamppb.New(t.Get())
	}

	var cipdPins *apipb.CipdPins
	if pinsPb := p.CIPDPins.ToProto(); pinsPb != nil {
		cipdPins = &apipb.CipdPins{
			ClientPackage: pinsPb.GetClientPackage(),
			Packages:      pinsPb.GetPackages(),
		}
	}

	var missingCAS []*apipb.CASReference
	if len(p.MissingCAS) != 0 {
		missingCAS = make([]*apipb.CASReference, 0, len(p.MissingCAS))
		for _, cas := range p.MissingCAS {
			missingCAS = append(missingCAS, cas.ToProto())
		}
	}

	var missingCIPD []*apipb.CipdPackage
	if len(p.MissingCIPD) != 0 {
		missingCIPD = make([]*apipb.CipdPackage, 0, len(p.MissingCIPD))
		for _, pkg := range p.MissingCIPD {
			missingCIPD = append(missingCIPD, pkg.ToProto())
		}
	}

	return &apipb.TaskResultResponse{
		AbandonedTs:         timeToTimestampPB(p.Abandoned),
		BotDimensions:       p.BotDimensions.ToProto(),
		BotIdleSinceTs:      timeToTimestampPB(p.BotIdleSince),
		BotLogsCloudProject: p.BotLogsCloudProject,
		BotVersion:          p.BotVersion,
		CasOutputRoot:       p.CASOutputRoot.ToProto(),
		CipdPins:            cipdPins,
		CompletedTs:         timeToTimestampPB(p.Completed),
		CurrentTaskSlice:    int32(p.CurrentTaskSlice),
		Duration:            float32(p.DurationSecs.Get()),
		ExitCode:            p.ExitCode.Get(),
		Failure:             p.Failure,
		InternalFailure:     p.InternalFailure,
		MissingCas:          missingCAS,
		MissingCipd:         missingCIPD,
		ModifiedTs:          timestamppb.New(p.Modified),
		ResultdbInfo:        p.ResultDBInfo.ToProto(),
		ServerVersions:      p.ServerVersions,
		StartedTs:           timeToTimestampPB(p.Started),
		State:               p.State,
	}
}

// ResultDBInfo contains invocation ID of the task.
type ResultDBInfo struct {
	// Hostname is the ResultDB service hostname e.g. "results.api.cr.dev".
	Hostname string `gae:"hostname"`
	// Invocation is the ResultDB invocation name for the task, if any.
	Invocation string `gae:"invocation"`
}

// ToProto converts ResultDBInfo to an apipb.ResultDBInfo.
func (p *ResultDBInfo) ToProto() *apipb.ResultDBInfo {
	if p.Hostname == "" && p.Invocation == "" {
		return nil
	}
	return &apipb.ResultDBInfo{
		Hostname:   p.Hostname,
		Invocation: p.Invocation,
	}
}

// TaskResultSummary represents the overall result of a task.
//
// Parent is a TaskRequest. Key id is always 1.
//
// It is created (in PENDING state) as soon as the task is created and then
// mutated whenever the task changes its state. Its used by various task listing
// APIs.
type TaskResultSummary struct {
	// TaskResultCommon embeds most of result properties.
	TaskResultCommon

	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key identifies the task, see TaskResultSummaryKey().
	Key *datastore.Key `gae:"$key"`

	// BotID is ID of a bot that runs this task, if already known.
	BotID datastore.Optional[string, datastore.Unindexed] `gae:"bot_id"`

	// Created is a timestamp when the task was submitted.
	//
	// The index is used in task listing queries to order by this property.
	//
	// TODO(vadimsh): Is the index actually used? Entity keys encode the same
	// information.
	Created time.Time `gae:"created_ts"`

	// Tags are copied from the corresponding TaskRequest entity.
	//
	// The index is used in global task listing queries to filter by.
	Tags []string `gae:"tags"`

	// RequestName is copied from the corresponding TaskRequest entity.
	RequestName string `gae:"name,noindex"`

	// RequestUser is copied from the corresponding TaskRequest entity.
	RequestUser string `gae:"user,noindex"`

	// RequestPriority is copied from the corresponding TaskRequest entity.
	RequestPriority int64 `gae:"priority,noindex"`

	// RequestAuthenticated is copied from the corresponding TaskRequest entity.
	RequestAuthenticated identity.Identity `gae:"request_authenticated,noindex"`

	// RequestRealm is copied from the corresponding TaskRequest entity.
	RequestRealm string `gae:"request_realm,noindex"`

	// RequestPool is a pool the task is targeting.
	//
	// Derived from dimensions in the corresponding TaskRequest entity.
	RequestPool string `gae:"request_pool,noindex"`

	// RequestBotID is a concrete bot the task is targeting (if any).
	//
	// Derived from dimensions in the corresponding TaskRequest entity.
	RequestBotID string `gae:"request_bot_id,noindex"`

	// PropertiesHash is used to find duplicate tasks.
	//
	// It is set to a hash of properties of the task slice only when these
	// conditions are met:
	// - TaskSlice.TaskProperties.Idempotent is true.
	// - State is COMPLETED.
	// - Failure is false (i.e. ExitCode is 0).
	// - InternalFailure is false.
	//
	// The index is used to find an an existing duplicate task when submitting
	// a new task.
	PropertiesHash datastore.Optional[[]byte, datastore.Indexed] `gae:"properties_hash"`

	// TryNumber indirectly identifies if this task was dedupped.
	//
	// This field is leftover from when swarming had internal retries and for that
	// reason has a weird semantics. See https://crbug.com/1065101.
	//
	// Possible values:
	//   null: if the task is still pending, has expired or was canceled.
	//   1: if the task was assigned to a bot and either currently runs or has
	//      finished or crashed already.
	//   0: if the task was dedupped.
	//
	// The index is used in global task listing queries to find what tasks were
	// dedupped.
	TryNumber datastore.Nullable[int64, datastore.Indexed] `gae:"try_number"`

	// CostUSD is an approximate bot time cost spent executing this task.
	CostUSD float64 `gae:"costs_usd,noindex"` // legacy plural property name

	// CostSavedUSD is an approximate cost saved by deduping this task.
	CostSavedUSD float64 `gae:"cost_saved_usd,noindex"`

	// DedupedFrom is set if this task reused results of some previous task.
	//
	// See PropertiesHash for conditions when this is possible.
	//
	// DedupedFrom is a packed TaskRunResult key of the reused result. Note that
	// there's no TaskRunResult representing this task, since it didn't run.
	DedupedFrom string `gae:"deduped_from,noindex"`

	// ExpirationDelay is a delay from TaskRequest.ExpirationTS to the actual
	// expiry time.
	//
	// This is set at expiration process if the last task slice expired by
	// reaching its deadline. Unset if the last slice expired because there were
	// no bots that could run it.
	//
	// Exclusively for monitoring.
	ExpirationDelay datastore.Optional[float64, datastore.Unindexed] `gae:"expiration_delay"`
}

// ToProto converts the TaskResultSummary struct to an apipb.TaskResultResponse.
//
// Note: This function will not handle PerformanceStats due to the requirement
// of fetching another datastore entity. Please refer to PerformanceStats to
// fetch them.
func (p *TaskResultSummary) ToProto() *apipb.TaskResultResponse {
	pb := p.TaskResultCommon.toProto()
	pb.BotId = p.BotID.Get()
	pb.CostSavedUsd = float32(p.CostSavedUSD)
	pb.CostsUsd = p.CostsUSD()
	pb.CreatedTs = timestamppb.New(p.Created)
	pb.DedupedFrom = p.DedupedFrom
	pb.Name = p.RequestName
	pb.PerformanceStats = nil // will have to be filled in later
	pb.RunId = p.TaskRunID()
	pb.Tags = p.Tags
	pb.TaskId = RequestKeyToTaskID(p.TaskRequestKey(), AsRequest)
	pb.User = p.RequestUser
	return pb
}

// CostsUSD converts the costUSD in TaskResultSummary to a []float32 containing
// that costUSD.
func (p *TaskResultSummary) CostsUSD() []float32 {
	if p.CostUSD == 0 {
		return nil
	}
	return []float32{float32(p.CostUSD)}
}

// GetOutput returns the stdout content for the task.
func (p *TaskResultSummary) GetOutput(ctx context.Context, length, offset int64) ([]byte, error) {
	if p.StdoutChunks == 0 {
		return nil, nil
	}
	if length == 0 {
		// Fetch the whole content
		length = p.StdoutChunks*ChunkSize - offset
	}
	firstChunk := offset / ChunkSize
	end := offset + length
	lastChunk := (end + ChunkSize - 1) / ChunkSize
	if lastChunk > p.StdoutChunks {
		lastChunk = p.StdoutChunks
	}
	toGet := make([]*TaskOutputChunk, lastChunk-firstChunk)
	for i := firstChunk; i < lastChunk; i++ {
		toGet[i-firstChunk] = &TaskOutputChunk{Key: TaskOutputChunkKey(ctx, p.TaskRequestKey(), i)}
	}
	handleChunk := func(chunk []byte, e error) ([]byte, error) {
		switch {
		case errors.Is(e, datastore.ErrNoSuchEntity):
			return bytes.Repeat([]byte{'\x00'}, ChunkSize), nil
		case e != nil:
			return nil, e
		}
		r, err := zlib.NewReader(bytes.NewReader(chunk))
		if err != nil {
			return nil, errors.Annotate(err, "failed to get zlib reader").Err()
		}
		decompressed, err := io.ReadAll(r)
		if err != nil {
			_ = r.Close()
			return nil, errors.Annotate(err, "failed to read chunk").Err()
		}
		if err := r.Close(); err != nil {
			return nil, errors.Annotate(err, "failed to close zlib reader").Err()
		}
		return decompressed, nil
	}
	var chunks [][]byte
	err := datastore.Get(ctx, toGet)
	for i := 0; i < len(toGet); i++ {
		chunkErr := func(i int) error {
			if merr, ok := err.(errors.MultiError); ok {
				return merr[i]
			}
			return err
		}
		decmpChunk, err := handleChunk(toGet[i].Chunk, chunkErr(i))
		if err != nil {
			return nil, errors.Annotate(err, "failed to handle chunk").Err()
		}
		chunks = append(chunks, decmpChunk)
	}
	min := func(x, y int) int {
		if x < y {
			return x
		}
		return y
	}
	toReturn := bytes.Join(chunks, nil)
	startOffset := offset % ChunkSize
	endOffset := min(int(end-(firstChunk*ChunkSize)), len(toReturn))
	return toReturn[startOffset:endOffset], nil
}

// PerformanceStats fetches the performance stats from datastore.
func (p *TaskResultSummary) PerformanceStats(ctx context.Context) (*apipb.PerformanceStats, error) {
	ps := &PerformanceStats{Key: PerformanceStatsKey(ctx, p.TaskRequestKey())}
	err := datastore.Get(ctx, ps)
	if err != nil {
		return nil, err
	}
	return ps.ToProto()
}

// TaskAuthInfo is information about the task for ACL checks.
func (p *TaskResultSummary) TaskAuthInfo() acls.TaskAuthInfo {
	return acls.TaskAuthInfo{
		TaskID:    RequestKeyToTaskID(p.TaskRequestKey(), AsRequest),
		Realm:     p.RequestRealm,
		Pool:      p.RequestPool,
		BotID:     p.RequestBotID,
		Submitter: p.RequestAuthenticated,
	}
}

// TaskRequestKey returns the parent task request key or panics if it is unset.
func (p *TaskResultSummary) TaskRequestKey() *datastore.Key {
	key := p.Key.Parent()
	if key == nil || key.Kind() != "TaskRequest" {
		panic(fmt.Sprintf("invalid TaskResultSummary key %q", p.Key))
	}
	return key
}

// TaskRunID returns the packed RunResult key for a swarming task.
func (p *TaskResultSummary) TaskRunID() string {
	// Returning nil if there was no attempt made.
	if !p.TryNumber.IsSet() {
		return ""
	}
	return RequestKeyToTaskID(p.TaskRequestKey(), AsRunResult)
}

// TaskResultSummaryKey construct a summary key given a task request key.
func TaskResultSummaryKey(ctx context.Context, taskReq *datastore.Key) *datastore.Key {
	return datastore.NewKey(ctx, "TaskResultSummary", "", 1, taskReq)
}

// TaskRunResult contains result of an attempt to run a task on a bot.
//
// Parent is a TaskResultSummary. Key id is 1.
//
// Unlike TaskResultSummary it is created only when the task is assigned to a
// bot, i.e. existence of this entity means a bot claimed the task and started
// executing it.
type TaskRunResult struct {
	// TaskResultCommon embeds most of result properties.
	TaskResultCommon

	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key identifies the task, see TaskRunResultKey().
	Key *datastore.Key `gae:"$key"`

	// BotID is ID of a bot that runs this task attempt.
	//
	// The index is used in task listing queries to filter by a specific bot.
	BotID string `gae:"bot_id"`

	// CostUSD is an approximate bot time cost spent executing this task.
	CostUSD float64 `gae:"cost_usd,noindex"`

	// Killing is true if a user requested the task to be canceled while it is
	// already running.
	//
	// Setting this field eventually results in the task moving to KILLED state.
	// This transition also unsets Killing back to false.
	Killing bool `gae:"killing,noindex"`

	// DeadAfter specifies the time after which the bot executing this task
	// is considered dead.
	//
	// It is set after every ping from the bot if the task is in RUNNING state
	// and get unset once the task terminates.
	DeadAfter datastore.Optional[time.Time, datastore.Unindexed] `gae:"dead_after_ts"`
}

// TaskRunResultKey constructs a task run result key given a task request key.
func TaskRunResultKey(ctx context.Context, taskReq *datastore.Key) *datastore.Key {
	return datastore.NewKey(ctx, "TaskRunResult", "", 1, TaskResultSummaryKey(ctx, taskReq))
}

// TaskRequestKey returns the parent task request key or panics if it is unset.
func (p *TaskRunResult) TaskRequestKey() *datastore.Key {
	key := p.Key.Parent().Parent()
	if key == nil || key.Kind() != "TaskRequest" {
		panic(fmt.Sprintf("invalid TaskRunResult key %q", p.Key))
	}
	return key
}

// ToProto converts the TaskRunResult struct to an apipb.TaskResultResponse.
//
// Note: This function will not handle PerformanceStats due to the requirement
// of fetching another datastore entity. Please refer to PerformanceStats to
// fetch them.
func (p *TaskRunResult) ToProto() *apipb.TaskResultResponse {
	pb := p.TaskResultCommon.toProto()
	pb.BotId = p.BotID
	pb.CostSavedUsd = 0 // the task is running, it doesn't save any cost
	pb.CostsUsd = []float32{float32(p.CostUSD)}
	pb.CreatedTs = nil        // TODO
	pb.DedupedFrom = ""       // the task is running, not deduped
	pb.Name = ""              // TODO
	pb.PerformanceStats = nil // will need to be filled in later
	pb.RunId = RequestKeyToTaskID(p.TaskRequestKey(), AsRunResult)
	pb.Tags = nil // TODO
	pb.TaskId = RequestKeyToTaskID(p.TaskRequestKey(), AsRequest)
	pb.User = "" // TODO
	return pb
}

// PerformanceStats contains various timing and performance information about
// the task as reported by the bot.
type PerformanceStats struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key identifies the task, see PerformanceStatsKey.
	Key *datastore.Key `gae:"$key"`

	// BotOverheadSecs is the total overhead in seconds, summing overhead from all
	// sections defined below.
	BotOverheadSecs float64 `gae:"bot_overhead,noindex"`

	// CacheTrim is stats of cache trimming before the dependency installations.
	CacheTrim OperationStats `gae:"cache_trim,lsp,noindex"`

	// PackageInstallation is stats of installing CIPD packages before the task.
	PackageInstallation OperationStats `gae:"package_installation,lsp,noindex"`

	// NamedCachesInstall is stats of named cache mounting before the task.
	NamedCachesInstall OperationStats `gae:"named_caches_install,lsp,noindex"`

	// NamedCachesUninstall is stats of named cache unmounting after the task.
	NamedCachesUninstall OperationStats `gae:"named_caches_uninstall,lsp,noindex"`

	// IsolatedDownload is stats of CAS dependencies download before the task.
	IsolatedDownload CASOperationStats `gae:"isolated_download,lsp,noindex"`

	// IsolatedUpload is stats of CAS uploading operation after the task.
	IsolatedUpload CASOperationStats `gae:"isolated_upload,lsp,noindex"`

	// Cleanup is stats of work directory cleanup operation after the task.
	Cleanup OperationStats `gae:"cleanup,lsp,noindex"`
}

// ToProto converts the PerformanceStats struct to apipb.PerformanceStats.
func (p *PerformanceStats) ToProto() (*apipb.PerformanceStats, error) {
	isolatedDownload, err := p.IsolatedDownload.ToProto()
	if err != nil {
		return nil, err
	}
	isolatedUpload, err := p.IsolatedUpload.ToProto()
	if err != nil {
		return nil, err
	}
	return &apipb.PerformanceStats{
		BotOverhead:          float32(p.BotOverheadSecs),
		CacheTrim:            p.CacheTrim.ToProto(),
		Cleanup:              p.Cleanup.ToProto(),
		IsolatedDownload:     isolatedDownload,
		IsolatedUpload:       isolatedUpload,
		NamedCachesInstall:   p.NamedCachesInstall.ToProto(),
		NamedCachesUninstall: p.NamedCachesUninstall.ToProto(),
		PackageInstallation:  p.PackageInstallation.ToProto(),
	}, nil
}

// TaskRequestKey converts PerformanceStats key to a TaskRequest key.
func (p *PerformanceStats) TaskRequestKey() *datastore.Key {
	// It is safe to do multiple nested key.Parent calls
	// A PerformanceStats key has the following ancestry:
	// TaskRequest -> TaskResultSummary -> TaskRunResult -> PerformanceStats
	// Each call to key.Parent() will return nil if there are no more parents.
	// key.Parent() returns nil if it encounters a nil receiver.
	// If this occurs we have enountered a programming error and so we will
	// crash with a nice error message.
	reqKey := p.Key.Parent().Parent().Parent()
	if reqKey == nil || reqKey.Kind() != "TaskRequest" {
		panic(fmt.Sprintf("Could not derive TaskRequest from PerformanceStats key %q", p.Key))
	}
	return reqKey
}

// OperationStats is performance stats of a particular operation.
//
// Stored as a unindexed subentity of PerformanceStats entity.
type OperationStats struct {
	// DurationSecs is how long the operation ran.
	DurationSecs float64 `gae:"duration"`
}

// ToProto converts the OperationStats struct to apipb.OperationStats.
func (p *OperationStats) ToProto() *apipb.OperationStats {
	if p.DurationSecs == 0 {
		return nil
	}
	return &apipb.OperationStats{
		Duration: float32(p.DurationSecs),
	}
}

// CASOperationStats is performance stats of a CAS operation.
//
// Stored as a unindexed subentity of PerformanceStats entity.
type CASOperationStats struct {
	// DurationSecs is how long the operation ran.
	DurationSecs float64 `gae:"duration"`

	// InitialItems is the number of items in the cache before the operation.
	InitialItems int64 `gae:"initial_number_items"`

	// InitialSize is the total cache size before the operation.
	InitialSize int64 `gae:"initial_size"`

	// ItemsCold is a set of sizes of items that were downloaded or uploaded.
	//
	// It is encoded in a special way, see packedintset.Unpack.
	ItemsCold []byte `gae:"items_cold"`

	// ItemsHot is a set of sizes of items that were already present in the cache.
	//
	// It is encoded in a special way, see packedintset.Unpack.
	ItemsHot []byte `gae:"items_hot"`
}

// ToProto converts the CASOperationStats struct to apipb.CASOperationStats.
func (p *CASOperationStats) ToProto() (*apipb.CASOperationStats, error) {
	if reflect.DeepEqual(*p, CASOperationStats{}) {
		return nil, nil
	}
	resp := &apipb.CASOperationStats{
		Duration:           float32(p.DurationSecs),
		InitialNumberItems: int32(p.InitialItems),
		InitialSize:        p.InitialSize,
	}
	if p.ItemsCold != nil {
		resp.ItemsCold = p.ItemsCold
		itemsCold, err := packedintset.Unpack(p.ItemsCold)
		if err != nil {
			return nil, errors.Annotate(err, "unpacking cold items").Err()
		}
		resp.NumItemsCold = int64(len(itemsCold))
		sum := int64(0)
		for _, val := range itemsCold {
			sum += val
		}
		resp.TotalBytesItemsCold = int64(sum)
	}
	if p.ItemsHot != nil {
		resp.ItemsHot = p.ItemsHot
		itemsHot, err := packedintset.Unpack(p.ItemsHot)
		if err != nil {
			return nil, errors.Annotate(err, "unpacking hot items").Err()
		}
		resp.NumItemsHot = int64(len(itemsHot))
		sum := int64(0)
		for _, val := range itemsHot {
			sum += val
		}
		resp.TotalBytesItemsHot = int64(sum)
	}
	return resp, nil
}

// PerformanceStatsKey builds a PerformanceStats key given a task request key.
func PerformanceStatsKey(ctx context.Context, taskReq *datastore.Key) *datastore.Key {
	return datastore.NewKey(ctx, "PerformanceStats", "", 1, TaskRunResultKey(ctx, taskReq))
}

// TaskOutputChunk represents a chunk of the task console output.
//
// The number of such chunks per task is tracked in TaskRunResult.StdoutChunks.
// Each entity holds a compressed segment of the task output. For all entities
// except the last one, the uncompressed size of the segment is ChunkSize. That
// way it is always possible to figure out what chunk to use for a given output
// offset.
type TaskOutputChunk struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key identifies the task and the chunk index, see TaskOutputChunkKey.
	Key *datastore.Key `gae:"$key"`

	// Chunk is zlib-compressed chunk of the output.
	Chunk []byte `gae:"chunk,noindex"`

	// Gaps is a series of 2 integer pairs, which specifies the part that are
	// invalid. Normally it should be empty. All values are relative to the start
	// of this chunk offset.
	Gaps []int32 `gae:"gaps,noindex"`
}

// TaskOutputChunkKey builds a TaskOutputChunk key for the given chunk index.
func TaskOutputChunkKey(ctx context.Context, taskReq *datastore.Key, idx int64) *datastore.Key {
	return datastore.NewKey(ctx, "TaskOutputChunk", "", idx+1,
		datastore.NewKey(ctx, "TaskOutput", "", 1, TaskRunResultKey(ctx, taskReq)))
}

// TaskResultSummaryQuery prepares a query that fetches TaskResultSummary
// entities.
func TaskResultSummaryQuery() *datastore.Query {
	return datastore.NewQuery("TaskResultSummary")
}

// FilterTasksByTags limits a TaskResultSummary query to return tasks matching
// given tags filter.
//
// For complex filters this may split the query into multiple queries that need
// to run in parallel with their results merged. See SplitForQuery() in Filter
// for more details.
func FilterTasksByTags(q *datastore.Query, mode SplitMode, tags Filter) []*datastore.Query {
	return tags.Apply(q, "tags", mode)
}

// FilterTasksByCreationTime limits a TaskResultSummary or TaskRunResult query
// to return tasks created within the given time range [start, end).
//
// Works only for queries ordered by key. Returns an error if timestamps are
// outside of expected range (e.g. before BeginningOfTheWorld).
//
// Zero time means no limit on the corresponding side of the range.
func FilterTasksByCreationTime(ctx context.Context, q *datastore.Query, start, end time.Time) (*datastore.Query, error) {
	// Keys are ordered by timestamp. Transform the provided timestamps into keys
	// to filter entities by key. The inequalities are inverted because keys are
	// in reverse chronological order.
	if !start.IsZero() {
		startReqKey, err := TimestampToRequestKey(ctx, start, 0)
		if err != nil {
			return nil, errors.Annotate(err, "invalid start time").Err()
		}
		q = q.Lte("__key__", TaskResultSummaryKey(ctx, startReqKey))
	}
	if !end.IsZero() {
		endReqKey, err := TimestampToRequestKey(ctx, end, 0)
		if err != nil {
			return nil, errors.Annotate(err, "invalid end time").Err()
		}
		q = q.Gt("__key__", TaskResultSummaryKey(ctx, endReqKey))
	}
	return q, nil
}

// FilterTasksByState limits a TaskResultSummary query to return tasks in
// particular state.
func FilterTasksByState(q *datastore.Query, state apipb.StateQuery) *datastore.Query {
	switch state {
	case apipb.StateQuery_QUERY_PENDING:
		q = q.Eq("state", apipb.TaskState_PENDING)
	case apipb.StateQuery_QUERY_RUNNING:
		q = q.Eq("state", apipb.TaskState_RUNNING)
	case apipb.StateQuery_QUERY_PENDING_RUNNING:
		// This assumes that no task has the state INVALID. Since the enum values
		// for TaskState_PENDING = 32 and TaskState_RUNNING = 16, we can use Lte.
		// Otherwise we would have to use
		//   q = q.In("state", apipb.TaskState_PENDING, apipb.TaskState_RUNNING)
		// which would add more complexity to the query.
		q = q.Lte("state", apipb.TaskState_PENDING)
	case apipb.StateQuery_QUERY_COMPLETED:
		q = q.Eq("state", apipb.TaskState_COMPLETED)
	case apipb.StateQuery_QUERY_COMPLETED_SUCCESS:
		q = q.Eq("state", apipb.TaskState_COMPLETED).Eq("failure", false)
	case apipb.StateQuery_QUERY_COMPLETED_FAILURE:
		q = q.Eq("state", apipb.TaskState_COMPLETED).Eq("failure", true)
	case apipb.StateQuery_QUERY_EXPIRED:
		q = q.Eq("state", apipb.TaskState_EXPIRED)
	case apipb.StateQuery_QUERY_TIMED_OUT:
		q = q.Eq("state", apipb.TaskState_TIMED_OUT)
	case apipb.StateQuery_QUERY_BOT_DIED:
		q = q.Eq("state", apipb.TaskState_BOT_DIED)
	case apipb.StateQuery_QUERY_CANCELED:
		q = q.Eq("state", apipb.TaskState_CANCELED)
	case apipb.StateQuery_QUERY_DEDUPED:
		q = q.Eq("state", apipb.TaskState_COMPLETED).Eq("try_number", 0)
	case apipb.StateQuery_QUERY_KILLED:
		q = q.Eq("state", apipb.TaskState_KILLED)
	case apipb.StateQuery_QUERY_NO_RESOURCE:
		q = q.Eq("state", apipb.TaskState_NO_RESOURCE)
	case apipb.StateQuery_QUERY_CLIENT_ERROR:
		q = q.Eq("state", apipb.TaskState_CLIENT_ERROR)
	default:
		// default case is apipb.StateQuery_QUERY_ALL, where the query is unchanged.
	}
	return q
}

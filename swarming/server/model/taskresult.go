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
	"strings"
	"time"

	"github.com/klauspost/compress/zlib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/packedintset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cursor/cursorpb"
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

// emptyChunk is used as a placeholder for missing TaskOutputChunk entities.
var emptyChunk = bytes.Repeat([]byte{0}, ChunkSize)

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

// TaskMetricFields is used as metric fields in some metrics related to tasks.
type TaskMetricFields struct {
	SpecName     string // name of a job specification
	ProjectID    string // e.g. "chromium".
	SubprojectID string // e.g. "blink". Set to empty string if not used.
	Pool         string // e.g. "Chrome".
	RBE          string // RBE instance of the task or literal "none".
	DeviceType   string // value of "device_type" tag if requested via MetricFields(true)
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

	// Created is a precise timestamp when the task was submitted.
	//
	// Unlike the timestamp in the entity key, this one has microsecond precision.
	Created time.Time `gae:"created_ts,noindex"`

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

// NewTaskResultSummary returns the new TaskResultSummary for a TaskRequest.
func NewTaskResultSummary(ctx context.Context, req *TaskRequest, serverVersion string, now time.Time) *TaskResultSummary {
	return &TaskResultSummary{
		Key: TaskResultSummaryKey(ctx, req.Key),
		TaskResultCommon: TaskResultCommon{
			State:          apipb.TaskState_PENDING,
			Modified:       now,
			ServerVersions: []string{serverVersion},
		},
		Created:              req.Created,
		Tags:                 req.Tags,
		RequestName:          req.Name,
		RequestUser:          req.User,
		RequestPriority:      req.Priority,
		RequestAuthenticated: req.Authenticated,
		RequestRealm:         req.Realm,
		RequestPool:          req.Pool(),
		RequestBotID:         req.BotID(),
	}
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
//
// Returns at most `length` bytes (perhaps less if reading the end of the
// output). Returns empty output if the task hasn't run.
func (p *TaskResultSummary) GetOutput(ctx context.Context, offset, length int64) ([]byte, error) {
	if offset < 0 {
		panic("negative offset")
	}
	if length < 0 {
		panic("negative length")
	}

	if p.StdoutChunks == 0 || length == 0 {
		return nil, nil
	}

	// Get TaskRequest of the task that actually has the logs (handling potential
	// task deduplication). Note that p.StdoutChunks is copied from the
	// TaskRunResult when the deduplication is recorded, thus it's still OK to use
	// it even when the result is deduplicated.
	runID := p.TaskRunID()
	if runID == "" {
		return nil, nil // the task hasn't run
	}
	dedupedFromReq, err := TaskIDToRequestKey(ctx, runID)
	if err != nil {
		return nil, errors.Annotate(err, "unexpectedly malformed dedupped run ID %q", runID).Err()
	}

	// A range of chunks covering the requested interval.
	firstChunk := offset / ChunkSize
	if firstChunk >= p.StdoutChunks {
		return nil, nil // the offset is totally outside of the recorded range
	}
	lastChunk := (offset + length + ChunkSize - 1) / ChunkSize
	if lastChunk > p.StdoutChunks {
		lastChunk = p.StdoutChunks
	}

	toGet := make([]*TaskOutputChunk, lastChunk-firstChunk)
	for i := range toGet {
		toGet[i] = &TaskOutputChunk{
			Key: TaskOutputChunkKey(ctx, dedupedFromReq, firstChunk+int64(i)),
		}
	}

	var zr io.ReadCloser

	decompress := func(chunk []byte, out *bytes.Buffer) (int, error) {
		in := bytes.NewReader(chunk)

		// Reuse the reader when decoding chunks to avoid generating GC garbage.
		var err error
		if zr == nil {
			zr, err = zlib.NewReader(in)
		} else {
			err = zr.(zlib.Resetter).Reset(in, nil)
		}
		if err != nil {
			return 0, errors.Annotate(err, "opening zlib reader").Err()
		}
		defer func() { _ = zr.Close() }()

		n, err := io.Copy(out, zr)
		if err != nil {
			return 0, errors.Annotate(err, "decompressing").Err()
		}
		if err := zr.Close(); err != nil {
			return 0, errors.Annotate(err, "closing zlib reader").Err()
		}
		return int(n), nil
	}

	output := bytes.Buffer{}
	output.Grow(len(toGet) * ChunkSize)

	err = datastore.Get(ctx, toGet)
	for i, chunk := range toGet {
		var chunkErr error
		if merr, ok := err.(errors.MultiError); ok {
			chunkErr = merr[i]
		} else {
			chunkErr = err
		}
		if errors.Is(chunkErr, datastore.ErrNoSuchEntity) {
			output.Write(emptyChunk)
			continue
		}
		if chunkErr != nil {
			return nil, errors.Annotate(chunkErr, "fetching chunk #%d", i).Err()
		}
		size, err := decompress(chunk.Chunk, &output)
		if err != nil {
			return nil, errors.Annotate(err, "decompressing chunk #%d", i).Err()
		}
		// All chunks except the last one must have length ChunkSize, otherwise
		// reading by offset will not always work correctly. Check that.
		if i != len(toGet)-1 && size != ChunkSize {
			logging.Warningf(ctx, "Chunk %s has unexpected size %d", chunk.Key, size)
		}
	}

	raw := output.Bytes()

	// The last chunk may be present, but have very little data, resulting in the
	// requested offset being outside of the recorded range. Return empty output
	// in that case.
	start := int(offset % ChunkSize)
	if start >= len(raw) {
		return nil, nil
	}
	return raw[start:min(start+int(length), len(raw))], nil
}

// PerformanceStatsKey returns PerformanceStats entity key or nil.
//
// It returns a non-nil key if the performance stats entity can **potentially**
// exist (e.g. the task has finished running and reported its stats). It returns
// nil if the performance stats entity definitely doesn't exist.
//
// If the task was dedupped, returns the key of the performance stats of the
// original task that actually ran.
func (p *TaskResultSummary) PerformanceStatsKey(ctx context.Context) *datastore.Key {
	switch {
	case p.DedupedFrom != "":
		reqKey, err := TaskIDToRequestKey(ctx, p.DedupedFrom)
		if err != nil {
			logging.Errorf(ctx, "Broken DedupedFrom %q in %s", p.DedupedFrom, p.Key)
			return nil
		}
		return PerformanceStatsKey(ctx, reqKey)
	case !ranAndDidNotCrash(p.State):
		return nil
	default:
		return PerformanceStatsKey(ctx, p.TaskRequestKey())
	}
}

// TaskAuthInfo returns information about the task for ACL checks.
//
// This implements acls.Task.
//
// If the entity is very old (older than ~Oct 2023) and doesn't yet have
// RequestXXX fields populated, values are fetched from the corresponding
// TaskRequest entity. This should be rare and this code path is intentionally
// not optimized to avoid adding complexity.
//
// The fallback can be removed ~May 2025 (when all "very old" entities will
// expire and disappear from the datastore).
//
// See https://chromium.googlesource.com/infra/luci/luci-py/+/b50d4ba949cb25b7
func (p *TaskResultSummary) TaskAuthInfo(ctx context.Context) (*acls.TaskAuthInfo, error) {
	// If at least one RequestXXX is populated, then this is a recent enough
	// entity. Note that many (but not all) of these fields can legitimately be
	// missing in termination tasks.
	if p.RequestRealm != "" || p.RequestPool != "" || p.RequestBotID != "" || p.RequestAuthenticated != "" {
		return &acls.TaskAuthInfo{
			TaskID:    RequestKeyToTaskID(p.TaskRequestKey(), AsRequest),
			Realm:     p.RequestRealm,
			Pool:      p.RequestPool,
			BotID:     p.RequestBotID,
			Submitter: p.RequestAuthenticated,
		}, nil
	}
	// Fallback to fetching necessary data from the TaskRequest.
	logging.Infof(ctx, "Fetching TaskRequest to get TaskAuthInfo")
	tr := &TaskRequest{Key: p.TaskRequestKey()}
	if err := datastore.Get(ctx, tr); err != nil {
		return nil, errors.Annotate(err, "failed to fetch TaskRequest entity").Err()
	}
	return tr.TaskAuthInfo(ctx)
}

// TaskRequestKey returns the parent task request key or panics if it is unset.
func (p *TaskResultSummary) TaskRequestKey() *datastore.Key {
	key := p.Key.Parent()
	if key == nil || key.Kind() != "TaskRequest" {
		panic(fmt.Sprintf("invalid TaskResultSummary key %q", p.Key))
	}
	return key
}

// IsActive returns whether the task is either PENDING or RUNNING.
func (p *TaskResultSummary) IsActive() bool {
	return p.State == apipb.TaskState_PENDING || p.State == apipb.TaskState_RUNNING
}

// TaskRunID returns the packed TaskRunResult key of the actual execution.
//
// If the task was dedupped, it will be the key of the run that actually
// happened.
//
// It is empty string if the task hasn't run (e.g. still pending or has
// expired).
func (p *TaskResultSummary) TaskRunID() string {
	switch {
	case p.DedupedFrom != "":
		return p.DedupedFrom
	case p.TryNumber.IsSet():
		return RequestKeyToTaskID(p.TaskRequestKey(), AsRunResult)
	default:
		return ""
	}
}

// PendingNow returns the duration the task spent pending to be scheduled as of now.
//
// If the duration is less than 0, return 0.
// If the task is deduped, return 0 and deduped as true.
func (p *TaskResultSummary) PendingNow(ctx context.Context, now time.Time) (diff time.Duration, deduped bool) {
	if p.DedupedFrom != "" {
		return 0, true
	}
	if started := p.Started.Get(); !started.IsZero() {
		now = started
	}

	diff = now.Sub(p.Created)
	if diff < 0 {
		logging.Warningf(ctx, "Pending time %s (%s - %s) should not be negative", diff, now, p.Created)
		diff = 0
	}
	return diff, false
}

// MetricFields returns metric fields to use to report metrics about this task.
func (p *TaskResultSummary) MetricFields(withDeviceType bool) TaskMetricFields {
	// This function is called a lot in the job scanning loop when collecting
	// statistics about active tasks. Avoid allocating memory to make it slightly
	// cheaper. Just scan the list of tags manually instead of converting them to
	// a map and then throwing it away.
	var (
		buildername         string
		buildIsExperimental string
		deviceType          string
		pool                string
		project             string
		rbe                 string
		specName            string
		subproject          string
		swarmingTerminate   string
		terminate           string
	)
	for _, t := range p.Tags {
		switch k, v, _ := strings.Cut(t, ":"); k {
		case "buildername":
			buildername = v
		case "build_is_experimental":
			buildIsExperimental = v
		case "device_type":
			deviceType = v
		case "pool":
			pool = v
		case "project":
			project = v
		case "rbe":
			rbe = v
		case "spec_name":
			specName = v
		case "subproject":
			subproject = v
		case "swarming.terminate":
			swarmingTerminate = v
		case "terminate":
			terminate = v
		}
	}

	if specName == "" {
		specName = buildername
		if buildIsExperimental == "true" {
			specName += ":experimental"
		}
		if specName == "" {
			if terminate == "1" || swarmingTerminate == "1" {
				specName = "swarming:terminate"
			}
		}
	}

	if rbe == "" {
		rbe = "none"
	}
	if !withDeviceType {
		deviceType = ""
	}

	return TaskMetricFields{
		SpecName:     specName,
		ProjectID:    project,
		SubprojectID: subproject,
		Pool:         pool,
		RBE:          rbe,
		DeviceType:   deviceType,
	}
}

// TaskResultSummaryKey construct a summary key given a task request key.
func TaskResultSummaryKey(ctx context.Context, taskReq *datastore.Key) *datastore.Key {
	return datastore.NewKey(ctx, "TaskResultSummary", "", 1, taskReq)
}

// TaskResultSummaryFromID returns a TaskResultSummary entity given a task id.
//
// Returns a grpc error if there's an issue.
func TaskResultSummaryFromID(ctx context.Context, id string) (*TaskResultSummary, error) {
	if id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "task_id is required")
	}

	key, err := TaskIDToRequestKey(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "task_id %s: %s", id, err)
	}

	trs := &TaskResultSummary{Key: TaskResultSummaryKey(ctx, key)}
	switch err = datastore.Get(ctx, trs); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "no such task")
	case err != nil:
		logging.Errorf(ctx, "Error fetching TaskResultSummary %s: %s", id, err)
		return nil, status.Errorf(codes.Internal, "datastore error fetching the task")
	}
	return trs, nil
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

	// RequestCreated is copied from the corresponding TaskRequest entity.
	RequestCreated time.Time `gae:"request_created,noindex"`

	// RequestTags are copied from the corresponding TaskRequest entity.
	RequestTags []string `gae:"request_tags,noindex"`

	// RequestName is copied from the corresponding TaskRequest entity.
	RequestName string `gae:"request_name,noindex"`

	// RequestUser is copied from the corresponding TaskRequest entity.
	RequestUser string `gae:"request_user,noindex"`

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
//
// This is purely a key constructor. It doesn't fetch anything and doesn't
// handle deduplicated tasks.
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

// PerformanceStatsKey returns PerformanceStats entity key or nil.
//
// It returns a non-nil key if the performance stats entity can **potentially**
// exist (e.g. the task has finished running and reported its stats). It returns
// nil if the performance stats entity definitely doesn't exist.
func (p *TaskRunResult) PerformanceStatsKey(ctx context.Context) *datastore.Key {
	if !ranAndDidNotCrash(p.State) {
		return nil
	}
	return PerformanceStatsKey(ctx, p.TaskRequestKey())
}

// ToProto converts the TaskRunResult struct to an apipb.TaskResultResponse.
//
// Note: This function will not handle PerformanceStats due to the requirement
// of fetching another datastore entity. Please refer to PerformanceStats to
// fetch them.
//
// TODO(2026): Older entities do not have some fields populated (in particular
// Name). The caller will have to fetch them separately from the corresponding
// TaskRequest entity. See TaskRunResultMissingFieldsFromRequest. This fallback
// can be removed in ~2026.
func (p *TaskRunResult) ToProto() *apipb.TaskResultResponse {
	pb := p.TaskResultCommon.toProto()
	pb.BotId = p.BotID
	pb.CostSavedUsd = 0 // the task is running, it doesn't save any cost
	pb.CostsUsd = []float32{float32(p.CostUSD)}
	pb.DedupedFrom = ""       // the task is running, not deduped
	pb.PerformanceStats = nil // will need to be filled in later
	pb.RunId = RequestKeyToTaskID(p.TaskRequestKey(), AsRunResult)
	pb.TaskId = RequestKeyToTaskID(p.TaskRequestKey(), AsRequest)
	if p.RequestName != "" {
		pb.CreatedTs = timestamppb.New(p.RequestCreated)
		pb.Name = p.RequestName
		pb.Tags = p.RequestTags
		pb.User = p.RequestUser
	}
	return pb
}

// TaskRunResultMissingFieldsFromRequest sets fields that TaskRunResult.ToProto
// could not set if the entity is too old.
func TaskRunResultMissingFieldsFromRequest(pb *apipb.TaskResultResponse, req *TaskRequest) {
	pb.CreatedTs = timestamppb.New(req.Created)
	pb.Name = req.Name
	pb.Tags = req.Tags
	pb.User = req.User
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

// PerformanceStatsKey builds a PerformanceStats key given a task request key.
func PerformanceStatsKey(ctx context.Context, taskReq *datastore.Key) *datastore.Key {
	return datastore.NewKey(ctx, "PerformanceStats", "", 1, TaskRunResultKey(ctx, taskReq))
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

// IsEmpty is true if this struct is unpopulated.
func (p *CASOperationStats) IsEmpty() bool {
	return p.DurationSecs == 0 && p.InitialItems == 0 && p.InitialSize == 0 &&
		len(p.ItemsCold) == 0 && len(p.ItemsHot) == 0
}

// ToProto converts the CASOperationStats struct to apipb.CASOperationStats.
func (p *CASOperationStats) ToProto() (*apipb.CASOperationStats, error) {
	if p.IsEmpty() {
		return nil, nil
	}
	resp := &apipb.CASOperationStats{
		Duration:           float32(p.DurationSecs),
		InitialNumberItems: int32(p.InitialItems),
		InitialSize:        p.InitialSize,
	}
	// TODO (crbug.com/329071986): Store precalculated sums in the stats. We only
	// need to compute sum once since ItemsCold and ItemsHot never change once
	// they are set and they generally can be pretty big, costing more compute
	// power.
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

// TaskRunResultQuery prepares a query that fetches TaskRunResult entities.
func TaskRunResultQuery() *datastore.Query {
	return datastore.NewQuery("TaskRunResult")
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
// to return tasks created within the given time range [start, end), applying
// a task pagination cursor as well if given (since it also just modifies the
// query time range).
//
// Works only for queries ordered by key. Returns an error if timestamps are
// outside of expected range (e.g. before BeginningOfTheWorld).
//
// Timestamp precision is truncated to milliseconds. Zero time means no limit on
// the corresponding side of the range.
//
// Returns datastore.ErrNullQuery if the pagination cursor points exactly at the
// `start` end of the query's time range. This is rare, but possible. Such query
// will always produce no results (we've "seen" the last result already, since
// it is in the cursor) and can be skipped.
func FilterTasksByCreationTime(ctx context.Context, q *datastore.Query, start, end time.Time, cursor *cursorpb.TasksCursor) (*datastore.Query, error) {
	if start.IsZero() && end.IsZero() && cursor == nil {
		return q, nil
	}

	// Need to use correct key constructors to limit the key range **precisely**
	// to what is being asked. This is important for handling tasks that sit on
	// the edge of the filtered range.
	var keyCB func(context.Context, *datastore.Key) *datastore.Key
	f, err := q.Finalize()
	if err != nil {
		panic(fmt.Sprintf("weird query: %v", q))
	}
	switch f.Kind() {
	case "TaskResultSummary":
		keyCB = TaskResultSummaryKey
	case "TaskRunResult":
		keyCB = TaskRunResultKey
	default:
		panic(fmt.Sprintf("FilterTasksByCreationTime is used with a query over kind %q", f.Kind()))
	}

	// TimestampToRequestKey will do truncation to 1 ms precision. Do it earlier
	// to be able to reject time ranges smaller than 1 ms sooner.
	if !start.IsZero() {
		start = start.Truncate(time.Millisecond)
	}
	if !end.IsZero() {
		end = end.Truncate(time.Millisecond)
	}

	// Refuse reversed or empty time range, such request is likely a client error.
	// It will be more helpful if it fails rather than silently returns no data.
	if !start.IsZero() && !end.IsZero() && !start.Before(end) {
		return nil, errors.Reason("start time must be before the end time").Err()
	}

	// Datastore entities are ordered by key (smaller to larger). Tasks need to
	// be ordered by their creation timestamp (more recent tasks first, e.g.
	// larger timestamp to smaller timestamp). To make this work, a timestamp
	// encoded by the key is inverted via negation in TimestampToRequestKey.
	//
	// In other words, task entities are ordered by their *age*, younger to older,
	// since to calculate the age we also need to negate the timestamp. So think
	// of the entity ID as an "age" expressed in some weird int64 units
	// calculated relative to some weird epoch.
	//
	// To limit the query to return entities with timestamps in `[start, end)`
	// range we convert the interval ends to these "age" units and then swap them.
	//
	// E.g. `[start, end)` that looks like `[9:00, 11:00)` becomes
	// `(endAge, startAge]` that looks kind of like `(5h old, 7h old]` (assuming
	// "now" is 16:00). Conversion from the timestamp to age happens in
	// TimestampToRequestKey.
	var startReqKey, endReqKey *datastore.Key
	if !start.IsZero() {
		if startReqKey, err = TimestampToRequestKey(ctx, start, 0); err != nil {
			return nil, errors.Annotate(err, "invalid start time").Err()
		}
	}
	if !end.IsZero() {
		if endReqKey, err = TimestampToRequestKey(ctx, end, 0); err != nil {
			return nil, errors.Annotate(err, "invalid end time").Err()
		}
	}

	// In paginated listings, we page from "younger" entities to more and more
	// older ones, until hitting the "oldest" end of the requested time interval
	// (aka `start`). The cursor contains the last returned entity ID (which is
	// the last entity's "age"). To resume from the cursor, we need to query the
	// age interval `(lastSeenAge, startAge]`. Thus the ID from the cursor should
	// replace the left side of the "age" query interval, i.e. the side originally
	// derived from `end`.
	if cursor != nil {
		cursorKey := datastore.NewKey(ctx, "TaskRequest", "", cursor.LastTaskRequestEntityId, nil)
		if endReqKey == nil {
			// The original query had no "youngest" age limit, but now it is limited
			// by the cursor.
			endReqKey = cursorKey
		} else if cursorKey.IntID() > endReqKey.IntID() {
			// The cursor is pointing to an entity "older" than the current range,
			// narrow down the query. This is how the actual "pagination" happens.
			endReqKey = cursorKey
		} else {
			// This can happen if the query time range has changed between pages and
			// the new time range is now more restrictive than what cursor points to.
			// We do not allow that.
			return nil, errors.Reason("the cursor is outside of the requested time range").Err()
		}
		// If `start` was changed between calls, the range may be empty now. We do
		// not allow that.
		if startReqKey != nil && endReqKey.IntID() > startReqKey.IntID() {
			return nil, errors.Reason("the cursor is outside of the requested time range").Err()
		}
		// It is theoretically possible that `startReqKey` matches some real entity
		// and we used this entity as the last cursor (since `startReqKey` end of
		// the query interval is inclusive). When resuming from such cursor, we'll
		// end up with an impossible query `id > startReqKey && id <= startReqKey`.
		// Communicate this via datastore.ErrNullQuery.
		if startReqKey != nil && endReqKey.IntID() == startReqKey.IntID() {
			return nil, datastore.ErrNullQuery
		}
	}

	if endReqKey != nil {
		q = q.Gt("__key__", keyCB(ctx, endReqKey))
	}
	if startReqKey != nil {
		q = q.Lte("__key__", keyCB(ctx, startReqKey))
	}

	return q, nil
}

// FilterTasksByTimestampField orders the query by a timestamp stored in the
// given field, filtering based on it as well.
//
// Tasks will be returned in the descending order (i.e. the most recent first).
//
// The filter range is [start, end). Zero time means no limit on the
// corresponding side of the range.
func FilterTasksByTimestampField(q *datastore.Query, field string, start, end time.Time) *datastore.Query {
	q = q.Order("-" + field)
	if !start.IsZero() {
		q = q.Gte(field, start)
	}
	if !end.IsZero() {
		q = q.Lt(field, end)
	}
	return q
}

// FilterTasksByState limits a TaskResultSummary query to return tasks in
// particular state.
//
// For StateQuery_QUERY_PENDING_RUNNING filter, depending on passed SplitMode,
// may either split the query into multiple queries that need to run in
// parallel, or append an "IN" filter to the original query. For all other state
// filters just adds simple "EQ" query filters.
//
// Per current datastore limitations (see Filter.SplitForQuery), a query can
// have at most one "IN" filter. Thus if FilterTasksByState returns a query with
// an "IN" filter, all queries built on top of it must not add any more "IN"
// filters. This is communicated by returning SplitCompletely SplitMode that
// should be applied to all subsequent splittings (if any).
func FilterTasksByState(q *datastore.Query, state apipb.StateQuery, splitMode SplitMode) ([]*datastore.Query, SplitMode) {
	// Special case that requires merging results of multiple queries.
	if state == apipb.StateQuery_QUERY_PENDING_RUNNING {
		// Note that it is tempting to use `q.Lte("state", apipb.TaskState_PENDING)`
		// since TaskState_RUNNING < TaskState_PENDING and there are no other states
		// like that. But Datastore allows an inequality filter on at least one
		// property. Queries that use `q.Lte("state", ...)` won't be able to use
		// an inequality filter for filtering by timestamp. So we are forced to use
		// `In("state", ...)` query (if supported) or merge to parallel queries
		// (when In queries are not supported, e.g. when running in dev mode). This
		// is communicated by splitMode.
		switch splitMode {
		case SplitOptimally:
			// It is OK to use an IN filter.
			return []*datastore.Query{
				q.In("state", apipb.TaskState_RUNNING, apipb.TaskState_PENDING),
			}, SplitCompletely
		case SplitCompletely:
			// Asked to avoid IN filters. Run two queries.
			return []*datastore.Query{
				q.Eq("state", apipb.TaskState_RUNNING),
				q.Eq("state", apipb.TaskState_PENDING),
			}, SplitCompletely
		default:
			panic(fmt.Sprintf("unexpected SplitMode %d", splitMode))
		}
	}

	switch state {
	case apipb.StateQuery_QUERY_PENDING:
		q = q.Eq("state", apipb.TaskState_PENDING)
	case apipb.StateQuery_QUERY_RUNNING:
		q = q.Eq("state", apipb.TaskState_RUNNING)
	case apipb.StateQuery_QUERY_PENDING_RUNNING:
		panic("already handled above")
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
	case apipb.StateQuery_QUERY_ALL:
		// No restrictions.
	default:
		panic(fmt.Sprintf("unexpected StateQuery %q", state))
	}
	return []*datastore.Query{q}, splitMode
}

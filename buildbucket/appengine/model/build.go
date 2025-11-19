// Copyright 2020 The LUCI Authors.
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
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	bb "go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/common/buildcel"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	// BuildKind is a Build entity's kind in the datastore.
	BuildKind = "Build"

	// BuildStatusKind is a BuildStatus entity's kind in the datastore.
	BuildStatusKind = "BuildStatus"

	// BuildStorageDuration is the maximum lifetime of a Build.
	//
	// Lifetime is the time elapsed since the Build creation time.
	// Cron runs periodically to scan and remove all the Builds of which
	// lifetime exceeded this duration.
	BuildStorageDuration = time.Hour * 24 * 30 * 18 // ~18 months
	// BuildMaxCompletionTime defines the maximum duration that a Build must be
	// completed within, from the build creation time.
	BuildMaxCompletionTime = time.Hour * 24 * 5 // 5 days

	// defaultBuildSyncInterval is the default interval between a build's latest
	// update time and the next time to sync it with backend.
	defaultBuildSyncInterval = 5 * time.Minute

	syncTimeSep = "--"
)

// isHiddenTag returns whether the given tag should be hidden by ToProto.
func isHiddenTag(key string) bool {
	// build_address is reserved by the server so that the TagIndex infrastructure
	// can be reused to fetch builds by builder + number (see tagindex.go and
	// rpc/get_build.go).
	// TODO(crbug/1042991): Unhide builder and gitiles_ref.
	// builder and gitiles_ref are allowed to be specified, are not internal,
	// and are only hidden here to match Python behavior.
	return key == "build_address" || key == "builder" || key == "gitiles_ref"
}

// PubSubCallback encapsulates parameters for a Pub/Sub callback.
type PubSubCallback struct {
	AuthToken string `gae:"auth_token,noindex"`
	Topic     string `gae:"topic,noindex"`
	UserData  []byte `gae:"user_data,noindex"`
}

// CustomMetric encapsulates information of one custom metric this build
// may report to.
type CustomMetric struct {
	Base   pb.CustomMetricBase        `gae:"base,noindex"`
	Metric *pb.CustomMetricDefinition `gae:"metric"`
}

// Build is a representation of a build in the datastore.
// Implements datastore.PropertyLoadSaver.
type Build struct {
	_     datastore.PropertyMap `gae:"-,extra"`
	_kind string                `gae:"$kind,Build"`
	ID    int64                 `gae:"$id"`

	// LegacyProperties are properties set for v1 legacy builds.
	LegacyProperties
	// UnusedProperties are properties set previously but currently unused.
	UnusedProperties

	// Proto is the pb.Build proto representation of the build.
	//
	// infra, input.properties, output.properties, and steps
	// are zeroed and stored in separate datastore entities
	// due to their potentially large size (see details.go).
	// tags are given their own field so they can be indexed.
	//
	// noindex is not respected here, it's set in pb.Build.ToProperty.
	Proto *pb.Build `gae:"proto,legacy"`

	Project string `gae:"project"`
	// <project>/<bucket>. Bucket is in v2 format.
	// e.g. chromium/try (never chromium/luci.chromium.try).
	BucketID string `gae:"bucket_id"`
	// <project>/<bucket>/<builder>. Bucket is in v2 format.
	// e.g. chromium/try/linux-rel.
	BuilderID string `gae:"builder_id"`

	Canary bool `gae:"canary"`

	CreatedBy identity.Identity `gae:"created_by"`
	// TODO(nodir): Replace reliance on create_time indices with id.
	CreateTime time.Time `gae:"create_time"`
	// Experimental, if true, means to exclude from monitoring and search results
	// (unless specifically requested in search results).
	Experimental bool `gae:"experimental"`
	// Experiments is a slice of experiments enabled or disabled on this build.
	// Each element should look like "[-+]$experiment_name".
	//
	// Special case:
	//   "-luci.non_production" is not kept here as a storage/index
	//   optimization.
	//
	//   Notably, all search/query implementations on the Build model
	//   apply this filter in post by checking that
	//   `b.ExperimentStatus("luci.non_production") == pb.Trinary_YES`.
	//
	//   This is because directly including this value in the datastore query
	//   results in bad performance due to excessive zig-zag join overhead
	//   in the datastore, since 99%+ of the builds in Buildbucket are production
	//   builds.
	Experiments []string `gae:"experiments"`
	Incomplete  bool     `gae:"incomplete"`

	// Deprecated; remove after v1 api turndown
	IsLuci bool `gae:"is_luci"`

	ResultDBUpdateToken string    `gae:"resultdb_update_token,noindex"`
	Status              pb.Status `gae:"status_v2"`
	StatusChangedTime   time.Time `gae:"status_changed_time"`
	// Tags is a slice of "<key>:<value>" strings taken from Proto.Tags.
	// Stored separately in order to index.
	Tags []string `gae:"tags"`

	// UpdateToken is set at the build creation time, and UpdateBuild requests are required
	// to have it in the header.
	UpdateToken string `gae:"update_token,noindex"`

	// StartBuildToken is set when a backend task starts, and StartBuild requests are required
	// to have it in the header.
	StartBuildToken string `gae:"start_build_token,noindex"`

	// PubSubCallback, if set, creates notifications for build status changes.
	PubSubCallback PubSubCallback `gae:"pubsub_callback,noindex"`

	// ParentID is the build's immediate parent build id.
	// Stored separately from AncestorIds in order to index this special case.
	ParentID int64 `gae:"parent_id"`

	// Ids of the build’s ancestors. This includes all parents/grandparents/etc.
	// This is ordered from top-to-bottom so `ancestor_ids[0]` is the root of
	// the builds tree, and `ancestor_ids[-1]` is this build's immediate parent.
	// This does not include any "siblings" at higher levels of the tree, just
	// the direct chain of ancestors from root to this build.
	AncestorIds []int64 `gae:"ancestor_ids"`

	// Id of the first StartBuildTask call Buildbucket receives for the build.
	// Buildbucket uses this to deduplicate the other StartBuildTask calls.
	StartBuildTaskRequestID string `gae:"start_task_request_id,noindex"`

	// Id of the first StartBuild call Buildbucket receives for the build.
	// Buildbucket uses this to deduplicate the other StartBuild calls.
	StartBuildRequestID string `gae:"start_build_request_id,noindex"`

	// Computed field to be used by a cron job to get the builds that have not
	// been updated for a while.
	//
	// It has a format like "<backend>--<project>--<shard>-=<next-sync-time>", where
	//   * backend is the backend target.
	//	 * project is the luci project of the build.
	//   * shard is the added prefix to make sure the index on this property is
	//     sharded to avoid hot spotting.
	//   * next-sync-time is the unix time of the next time the build is supposed
	//     to be synced with its backend task, truncated in minute.
	NextBackendSyncTime string `gae:"next_backend_sync_time"`

	// Backend target for builds on TaskBackend.
	BackendTarget string `gae:"backend_target"`

	// How far into the future should NextBackendSyncTime be set after a build update.
	BackendSyncInterval time.Duration `gae:"backend_sync_interval,noindex"`

	// The list of custom metrics that the build may report to.
	// Copied from the builder config when the build is created.
	CustomMetrics []CustomMetric `gae:"custome_metrics,noindex"`

	// Names of the custom builder metrics this build could report to.
	// Each base has a separate field for custom builder metrics based on it.
	// We report builder metrics by querying datastore, so doing predicate checks
	// on query results is infeasible or expensive. Instead we should do the
	// predicate check when an event happens then saves the metric name if the
	// check passes. So later we could run a query for that specific custom metric.
	CustomBuilderCountMetrics               []string `gae:"custom_builder_count_metrics"`
	CustomBuilderMaxAgeMetrics              []string `gae:"custom_builder_max_age_metrics"`
	CustomBuilderConsecutiveFailuresMetrics []string `gae:"custom_builder_consecutive_failures_metrics"`

	// TurboCI related fields.

	// For Turbo CI builds, `ids.v1.StageAttempt` serialized as a Turbo CI ID.
	StageAttemptID string `gae:"stage_attempt_id"`
	// A token that can be used to write to the Turbo CI work plan as this stage.
	StageAttemptToken string `gae:"stage_attempt_token,noindex"`
}

// Realm returns this build's auth realm, or an empty string if not opted into the
// realms experiment.
func (b *Build) Realm() string {
	return fmt.Sprintf("%s:%s", b.Proto.Builder.Project, b.Proto.Builder.Bucket)
}

// ExperimentStatus scans the experiments attached to this Build and returns:
//   - YES - The experiment was known at schedule time and enabled.
//   - NO - The experiment was known at schedule time and disabled.
//   - UNSET - The experiment was unknown at schedule time.
//
// Malformed Experiment filters are treated as UNSET.
func (b *Build) ExperimentStatus(expname string) (ret pb.Trinary) {
	b.IterExperiments(func(enabled bool, exp string) bool {
		if exp == expname {
			if enabled {
				ret = pb.Trinary_YES
			} else {
				ret = pb.Trinary_NO
			}
			return false
		}
		return true
	})
	return
}

// IterExperiments parses all experiments and calls `cb` for each.
//
// This will always include a call with bb.ExperimentNonProduction, even
// if '-'+bb.ExperimentNonProduction isn't recorded in the underlying
// Experiments field.
func (b *Build) IterExperiments(cb func(enabled bool, exp string) bool) {
	var hadNonProd bool

	for _, expFilter := range b.Experiments {
		if len(expFilter) == 0 {
			continue
		}
		plusMinus, exp := expFilter[0], expFilter[1:]
		hadNonProd = hadNonProd || exp == bb.ExperimentNonProduction

		keepGoing := true
		if plusMinus == '+' {
			keepGoing = cb(true, exp)
		} else if plusMinus == '-' {
			keepGoing = cb(false, exp)
		}
		if !keepGoing {
			return
		}
	}
	if !hadNonProd {
		cb(false, bb.ExperimentNonProduction)
	}
}

// ExperimentsString sorts, joins, and returns the enabled experiments with "|".
//
// Returns "None" if no experiments were enabled in the build.
func (b *Build) ExperimentsString() string {
	if len(b.Experiments) == 0 {
		return "None"
	}

	enables := make([]string, 0, len(b.Experiments))
	b.IterExperiments(func(isEnabled bool, name string) bool {
		if isEnabled {
			enables = append(enables, name)
		}
		return true
	})
	if len(enables) > 0 {
		sort.Strings(enables)
		return strings.Join(enables, "|")
	}
	return "None"
}

// Load overwrites this representation of a build by reading the given
// datastore.PropertyMap. Mutates this entity.
func (b *Build) Load(p datastore.PropertyMap) error {
	return datastore.GetPLS(b).Load(p)
}

// Save returns the datastore.PropertyMap representation of this build. Mutates
// this entity to reflect computed datastore fields in the returned PropertyMap.
func (b *Build) Save(withMeta bool) (datastore.PropertyMap, error) {
	b.BucketID = protoutil.FormatBucketID(b.Proto.Builder.Project, b.Proto.Builder.Bucket)
	b.BuilderID = protoutil.FormatBuilderID(b.Proto.Builder)
	b.Canary = b.Proto.Canary
	b.Experimental = b.Proto.Input.GetExperimental()
	b.Incomplete = !protoutil.IsEnded(b.Proto.Status)
	b.Project = b.Proto.Builder.Project

	oldStatus := b.Status
	b.Status = b.Proto.Status
	if b.Status != oldStatus {
		b.StatusChangedTime = b.Proto.UpdateTime.AsTime()
	}
	b.CreateTime = b.Proto.CreateTime.AsTime()

	if b.BackendTarget != "" && b.BackendSyncInterval == 0 {
		b.BackendSyncInterval = defaultBuildSyncInterval
	}

	if b.NextBackendSyncTime != "" && b.Proto.UpdateTime != nil {
		backend, project, shardID, oldUnix := b.MustParseNextBackendSyncTime()
		newUnix := fmt.Sprint(b.calculateNextSyncTime().Unix())
		if newUnix > oldUnix {
			b.NextBackendSyncTime = strings.Join([]string{backend, project, shardID, newUnix}, syncTimeSep)
		}
	}

	// Set legacy values used by Python.
	switch b.Status {
	case pb.Status_SCHEDULED:
		b.LegacyProperties.Result = 0
		b.LegacyProperties.Status = Scheduled
	case pb.Status_STARTED:
		b.LegacyProperties.Result = 0
		b.LegacyProperties.Status = Started
	case pb.Status_SUCCESS:
		b.LegacyProperties.Result = Success
		b.LegacyProperties.Status = Completed
	case pb.Status_FAILURE:
		b.LegacyProperties.FailureReason = BuildFailure
		b.LegacyProperties.Result = Failure
		b.LegacyProperties.Status = Completed
	case pb.Status_INFRA_FAILURE:
		if b.Proto.StatusDetails.GetTimeout() != nil {
			b.LegacyProperties.CancelationReason = TimeoutCanceled
			b.LegacyProperties.Result = Canceled
		} else {
			b.LegacyProperties.FailureReason = InfraFailure
			b.LegacyProperties.Result = Failure
		}
		b.LegacyProperties.Status = Completed
	case pb.Status_CANCELED:
		b.LegacyProperties.CancelationReason = ExplicitlyCanceled
		b.LegacyProperties.Result = Canceled
		b.LegacyProperties.Status = Completed
	}

	b.AncestorIds = b.Proto.AncestorIds
	if len(b.Proto.AncestorIds) > 0 {
		b.ParentID = b.Proto.AncestorIds[len(b.Proto.AncestorIds)-1]
	}

	p, err := datastore.GetPLS(b).Save(withMeta)
	if err != nil {
		return nil, err
	}

	// Parameters and ResultDetails are only set via v1 API which is unsupported in
	// Go. In order to preserve the value of these fields without having to interpret
	// them, the type is set to []byte. But if no values for these fields are set,
	// the []byte type causes an empty-type specific value (i.e. empty string) to be
	// written to the datastore. Since Python interprets these fields as JSON, and
	// and an empty string is not a valid JSON object, convert empty strings to nil.
	// TODO(crbug/1042991): Remove Properties default once v1 API is removed.
	if len(b.LegacyProperties.Parameters) == 0 {
		p["parameters"] = datastore.MkProperty(nil)
	}
	// TODO(crbug/1042991): Remove ResultDetails default once v1 API is removed.
	if len(b.LegacyProperties.ResultDetails) == 0 {
		p["result_details"] = datastore.MkProperty(nil)
	}

	// Writing a value for PubSubCallback confuses the Python implementation which
	// expects PubSubCallback to be a LocalStructuredProperty. See also unused.go.
	delete(p, "pubsub_callback")
	return p, nil
}

// ToProto returns the *pb.Build representation of this build after applying the
// provided mask and redaction function.
func (b *Build) ToProto(ctx context.Context, m *BuildMask, redact func(*pb.Build) error) (*pb.Build, error) {
	build := b.ToSimpleBuildProto(ctx)
	if err := LoadBuildDetails(ctx, m, redact, build); err != nil {
		return nil, err
	}
	return build, nil
}

// ToDryRunProto returns the *pb.Build representation of this build when
// creating it in DryRun mode: such builds don't have an ID and they (nor
// any of their details) are stored in the datastore.
func (b *Build) ToDryRunProto(ctx context.Context, m *BuildMask) (*pb.Build, error) {
	// TODO: This ignores `m` to be compatible with historical ScheduleBuild
	// implementation that ignored the mask here, see
	// https: //chromium.googlesource.com/infra/luci/luci-go/+/66e2decbc/buildbucket/appengine/rpc/schedule_build.go#1959
	return b.Proto, nil
}

// ToSimpleBuildProto returns the *pb.Build without loading steps, infra,
// input/output properties. Unlike ToProto, does not support redaction of fields.
func (b *Build) ToSimpleBuildProto(ctx context.Context) *pb.Build {
	p := proto.Clone(b.Proto).(*pb.Build)
	p.Tags = make([]*pb.StringPair, 0, len(b.Tags))
	for _, t := range b.Tags {
		k, v := strpair.Parse(t)
		if !isHiddenTag(k) {
			p.Tags = append(p.Tags, &pb.StringPair{
				Key:   k,
				Value: v,
			})
		}
	}
	return p
}

func (b *Build) GetParentID() int64 {
	if len(b.Proto.AncestorIds) > 0 {
		return b.Proto.AncestorIds[len(b.Proto.AncestorIds)-1]
	}
	return 0
}

// ClearLease clears the lease by resetting the LeaseProperties.
//
// NeverLeased is kept unchanged.
func (b *Build) ClearLease() {
	b.LeaseProperties = LeaseProperties{NeverLeased: b.LeaseProperties.NeverLeased}
}

// GenerateNextBackendSyncTime generates the build's NextBackendSyncTime if the build
// runs on a backend.
func (b *Build) GenerateNextBackendSyncTime(ctx context.Context, shards int32) {
	if b.BackendTarget == "" {
		return
	}

	shardID := 0
	if shards > 1 {
		seeded := rand.New(rand.NewSource(clock.Now(ctx).UnixNano()))
		shardID = seeded.Intn(int(shards))
	}
	b.NextBackendSyncTime = ConstructNextSyncTime(b.BackendTarget, b.Project, shardID, b.calculateNextSyncTime())
}

func ConstructNextSyncTime(backend, project string, shardID int, syncTime time.Time) string {
	return strings.Join([]string{backend, project, fmt.Sprint(shardID), fmt.Sprint(syncTime.Unix())}, syncTimeSep)
}

// calculateNextSyncTime calculates the next time the build should be synced.
// It rounds the build's update time to the closest minute them add BackendSyncInterval.
func (b *Build) calculateNextSyncTime() time.Time {
	return b.Proto.UpdateTime.AsTime().Round(time.Minute).Add(b.BackendSyncInterval)
}

func (b *Build) MustParseNextBackendSyncTime() (backend, project, shardID, syncTime string) {
	parts := strings.Split(b.NextBackendSyncTime, syncTimeSep)
	if len(parts) != 4 {
		panic(fmt.Sprintf("build.NextBackendSyncTime %s is in a wrong format", b.NextBackendSyncTime))
	}
	return parts[0], parts[1], parts[2], parts[3]
}

// LoadBuildDetails loads the details of the given builds, trimming them
// according to the specified mask and redaction function.
func LoadBuildDetails(ctx context.Context, m *BuildMask, redact func(*pb.Build) error, builds ...*pb.Build) error {
	l := len(builds)
	inf := make([]*BuildInfra, 0, l)
	inp := make([]*BuildInputProperties, 0, l)
	out := make([]*BuildOutputProperties, 0, l)
	stp := make([]*BuildSteps, 0, l)
	var dets []any

	included := map[string]bool{
		"infra":             m.Includes("infra"),
		"input.properties":  m.Includes("input.properties"),
		"output.properties": m.Includes("output.properties"),
		"steps":             m.Includes("steps"),
	}

	for i, p := range builds {
		if p.GetId() <= 0 {
			return errors.Fmt("invalid build for %q", p)
		}
		key := datastore.KeyForObj(ctx, &Build{ID: p.Id})
		inf = append(inf, &BuildInfra{Build: key})
		inp = append(inp, &BuildInputProperties{Build: key})
		out = append(out, &BuildOutputProperties{Build: key})
		stp = append(stp, &BuildSteps{Build: key})
		appendIfIncluded := func(path string, det any) {
			if included[path] {
				dets = append(dets, det)
			}
		}
		appendIfIncluded("infra", inf[i])
		appendIfIncluded("input.properties", inp[i])
		appendIfIncluded("steps", stp[i])
	}

	if err := GetIgnoreMissing(ctx, dets); err != nil {
		return errors.Fmt("error fetching build details: %w", err)
	}

	// For `output.properties`, should use *BuildOutputProperties.Get, instead of
	// using datastore.Get directly.
	if included["output.properties"] {
		if err := errors.Filter(GetMultiOutputProperties(ctx, out...), datastore.ErrNoSuchEntity); err != nil {
			return errors.Fmt("error fetching build(s) output properties: %w", err)
		}
	}

	var err error
	for i, p := range builds {
		p.Infra = inf[i].Proto
		if p.Input == nil {
			p.Input = &pb.Build_Input{}
		}
		p.Input.Properties = inp[i].Proto
		if p.Output == nil {
			p.Output = &pb.Build_Output{}
		}
		p.Output.Properties = out[i].Proto
		p.Steps, err = stp[i].ToProto(ctx)
		if err != nil {
			return errors.Fmt("error fetching steps for build %q: %w", p.Id, err)
		}
		if m.Includes("summary_markdown") {
			p.SummaryMarkdown = protoutil.MergeSummary(p)
		}
		if redact != nil {
			if err = redact(p); err != nil {
				return errors.Fmt("error redacting build %q: %w", p.Id, err)
			}
		}
		if err = m.Trim(p); err != nil {
			return errors.Fmt("error trimming fields for build %q: %w", p.Id, err)
		}
	}
	return nil
}

func getBuilderMetricBasesByStatus(status pb.Status) map[pb.CustomMetricBase][]*pb.CustomMetricDefinition {
	switch {
	case status == pb.Status_SCHEDULED:
		return map[pb.CustomMetricBase][]*pb.CustomMetricDefinition{
			pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT:             nil,
			pb.CustomMetricBase_CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED: nil,
		}
	case status == pb.Status_STARTED:
		return map[pb.CustomMetricBase][]*pb.CustomMetricDefinition{
			pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT: nil,
		}
	case protoutil.IsEnded(status):
		return map[pb.CustomMetricBase][]*pb.CustomMetricDefinition{
			pb.CustomMetricBase_CUSTOM_METRIC_BASE_CONSECUTIVE_FAILURE_COUNT: nil,
		}
	default:
		panic(fmt.Sprintf("invalid build status %s", status))
	}
}

// EvaluateBuildForCustomBuilderMetrics finds the custom builder metrics from
// bld.CustomMetrics and evaluates the build against those metrics' predicates.
// Update bld.CustomBuilderXXMetrics to add names of the matching metrics.
func EvaluateBuildForCustomBuilderMetrics(ctx context.Context, bld *Build, bp *pb.Build, loadDetails bool) error {
	toEvaluate := getBuilderMetricBasesByStatus(bld.Proto.Status)
	needToEvaluate := false
	for _, cm := range bld.CustomMetrics {
		if _, ok := toEvaluate[cm.Base]; ok {
			needToEvaluate = true
			toEvaluate[cm.Base] = append(toEvaluate[cm.Base], cm.Metric)
		}
	}
	if !needToEvaluate {
		return nil
	}

	// Load build details if needed.
	if bp == nil {
		bp = bld.Proto
		if loadDetails {
			// Use a copy of bld.Proto here since we need to load details to it.
			bp = bld.ToSimpleBuildProto(ctx)
			m, err := NewBuildMask("", nil, &pb.BuildMask{AllFields: true})
			if err != nil {
				return err
			}
			err = LoadBuildDetails(ctx, m, func(b *pb.Build) error { return nil }, bp)
			if err != nil {
				return err
			}
		}
	}

	// Evaluate the build.
	for base, cms := range toEvaluate {
		if base == pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT {
			// Clear bld.CustomBuilderCountMetrics to (re)evaluate all possible
			// builder metrics with this base at build updates (first evaluation
			// happens at build creation). Re-evaluating at build updates is cheap
			// since we don't need to load details of the build from datastore.
			//
			// No need to re-evaluate builder metrics with other bases, since they are
			// only evaluated once at either build creation or completion.
			bld.CustomBuilderCountMetrics = nil
		}

		for _, cm := range cms {
			matched, err := buildcel.BoolEval(bp, cm.Predicates)
			if err != nil {
				return err
			}
			if matched {
				switch base {
				case pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT:
					bld.CustomBuilderCountMetrics = append(bld.CustomBuilderCountMetrics, cm.Name)
				case pb.CustomMetricBase_CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED:
					bld.CustomBuilderMaxAgeMetrics = append(bld.CustomBuilderMaxAgeMetrics, cm.Name)
				case pb.CustomMetricBase_CUSTOM_METRIC_BASE_CONSECUTIVE_FAILURE_COUNT:
					bld.CustomBuilderConsecutiveFailuresMetrics = append(bld.CustomBuilderConsecutiveFailuresMetrics, cm.Name)
				default:
					panic(fmt.Sprintf("impossible builder metric base %s", base))
				}
			}
		}
	}
	return nil
}

// BuildStatus stores build ids and their statuses.
type BuildStatus struct {
	_kind string `gae:"$kind,BuildStatus"`

	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id,1"`

	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`

	// Address of a build.
	// * If build number is enabled for the build, the address would be
	// <project>/<bucket>/<builder>/<build_number>;
	// * otherwise the address would be <project>/<bucket>/<builder>/b<build_id> (
	// to easily differentiate build number and build id).
	BuildAddress string `gae:"build_address"`

	Status pb.Status `gae:"status,noindex"`
}

// Copyright 2022 The LUCI Authors.
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

package rpc

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	cipdCommon "go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/internal/resultdb"
	"go.chromium.org/luci/buildbucket/appengine/internal/search"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/appengine/tasks"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var casInstanceRe = regexp.MustCompile(`^projects/[^/]*/instances/[^/]*$`)

type CreateBuildChecker struct{}

var _ protowalk.FieldProcessor = (*CreateBuildChecker)(nil)

func (CreateBuildChecker) Process(_ protowalk.DataMap, field protoreflect.FieldDescriptor, msg protoreflect.Message) (data protowalk.ResultData, applied bool) {
	cbfb := proto.GetExtension(field.Options().(*descriptorpb.FieldOptions), pb.E_CreateBuildFieldOption).(*pb.CreateBuildFieldOption)
	switch cbfb.FieldBehavior {
	case annotations.FieldBehavior_OUTPUT_ONLY:
		msg.Clear(field)
		return protowalk.ResultData{Message: "cleared OUTPUT_ONLY field"}, true
	case annotations.FieldBehavior_REQUIRED:
		return protowalk.ResultData{Message: "required", IsErr: true}, true
	default:
		panic("unsupported field behavior")
	}
}

func (CreateBuildChecker) ShouldProcess(field protoreflect.FieldDescriptor) protowalk.ProcessAttr {
	if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
		if cbfb := proto.GetExtension(fo, pb.E_CreateBuildFieldOption).(*pb.CreateBuildFieldOption); cbfb != nil {
			switch cbfb.FieldBehavior {
			case annotations.FieldBehavior_OUTPUT_ONLY:
				return protowalk.ProcessIfSet
			case annotations.FieldBehavior_REQUIRED:
				return protowalk.ProcessIfUnset
			default:
				panic("unsupported field behavior")
			}
		}
	}
	return protowalk.ProcessNever
}

func validateBucketConstraints(ctx context.Context, b *pb.Build) error {
	bck := &model.Bucket{
		Parent: model.ProjectKey(ctx, b.Builder.Project),
		ID:     b.Builder.Bucket,
	}
	bckStr := fmt.Sprintf("%s:%s", b.Builder.Project, b.Builder.Bucket)
	if err := datastore.Get(ctx, bck); err != nil {
		return errors.Fmt("failed to fetch bucket config %s: %w", bckStr, err)
	}

	constraints := bck.Proto.GetConstraints()
	if constraints == nil {
		return errors.Fmt("constraints for %s not found", bckStr)
	}

	allowedPools := stringset.NewFromSlice(constraints.GetPools()...)
	allowedSAs := stringset.NewFromSlice(constraints.GetServiceAccounts()...)

	var pool string
	for _, dim := range taskDimensions(b.GetInfra()) {
		if dim.Key == "pool" {
			pool = dim.Value
			break
		}
	}
	if pool == "" || !allowedPools.Has(pool) {
		return errors.Fmt("pool: %s not allowed", pool)
	}

	sa := taskServiceAccount(b.GetInfra())
	if sa == "" || !allowedSAs.Has(sa) {
		return errors.Fmt("service_account: %s not allowed", sa)
	}
	return nil
}

func taskDimensions(infra *pb.BuildInfra) []*pb.RequestedDimension {
	if infra.GetBackend() != nil {
		return infra.GetBackend().GetTaskDimensions()
	}
	return infra.GetSwarming().GetTaskDimensions()
}

func taskServiceAccount(infra *pb.BuildInfra) string {
	if infra.GetBackend() != nil {
		sav := infra.GetBackend().GetConfig().GetFields()["service_account"]
		if v, ok := sav.GetKind().(*structpb.Value_StringValue); ok {
			return v.StringValue
		}
	}
	return infra.GetSwarming().GetTaskServiceAccount()
}

func validateHostName(host string) error {
	if strings.Contains(host, "://") {
		return errors.New(`must not contain "://"`)
	}
	return nil
}

func validateCipdPackage(pkg string, mustWithSuffix bool) error {
	pkgSuffix := "/${platform}"
	if mustWithSuffix && !strings.HasSuffix(pkg, pkgSuffix) {
		return errors.Fmt("expected to end with %s", pkgSuffix)
	}
	return cipdCommon.ValidatePackageName(strings.TrimSuffix(pkg, pkgSuffix))
}

func validateAgentInput(in *pb.BuildInfra_Buildbucket_Agent_Input) error {
	for path, ref := range in.GetData() {
		for i, spec := range ref.GetCipd().GetSpecs() {
			if err := validateCipdPackage(spec.GetPackage(), false); err != nil {
				return errors.Fmt("[%s]: [%d]: cipd.package: %w", path, i, err)
			}
			if err := cipdCommon.ValidateInstanceVersion(spec.GetVersion()); err != nil {
				return errors.Fmt("[%s]: [%d]: cipd.version: %w", path, i, err)
			}
		}

		cas := ref.GetCas()
		if cas != nil {
			switch {
			case !casInstanceRe.MatchString(cas.GetCasInstance()):
				return errors.Fmt("[%s]: cas.cas_instance: does not match %s", path, casInstanceRe)
			case cas.GetDigest() == nil:
				return errors.Fmt("[%s]: cas.digest: not specified", path)
			case cas.Digest.GetSizeBytes() < 0:
				return errors.Fmt("[%s]: cas.digest.size_bytes: must be greater or equal to 0", path)
			}
		}
	}
	return nil
}

func validateAgentSource(src *pb.BuildInfra_Buildbucket_Agent_Source) error {
	cipd := src.GetCipd()
	if err := validateCipdPackage(cipd.GetPackage(), true); err != nil {
		return errors.Fmt("cipd.package:: %w", err)
	}
	if err := cipdCommon.ValidateInstanceVersion(cipd.GetVersion()); err != nil {
		return errors.Fmt("cipd.version: %w", err)
	}
	return nil
}

func validateAgentPurposes(purposes map[string]pb.BuildInfra_Buildbucket_Agent_Purpose, in *pb.BuildInfra_Buildbucket_Agent_Input) error {
	if len(purposes) == 0 {
		return nil
	}

	for path := range purposes {
		if _, ok := in.GetData()[path]; !ok {
			return errors.Fmt("Invalid path %s - not in input dataRef", path)
		}
	}
	return nil
}

func validateAgent(agent *pb.BuildInfra_Buildbucket_Agent) error {
	var err error
	switch {
	case teeErr(validateAgentInput(agent.GetInput()), &err) != nil:
		return errors.Fmt("input: %w", err)
	case teeErr(validateAgentSource(agent.GetSource()), &err) != nil:
		return errors.Fmt("source: %w", err)
	case teeErr(validateAgentPurposes(agent.GetPurposes(), agent.GetInput()), &err) != nil:
		return errors.Fmt("purposes: %w", err)
	default:
		return nil
	}
}

func validateInfraBuildbucket(ctx context.Context, ib *pb.BuildInfra_Buildbucket) error {
	var err error
	bbHost := fmt.Sprintf("%s.appspot.com", info.AppID(ctx))
	switch {
	case teeErr(validateHostName(ib.GetHostname()), &err) != nil:
		return errors.Fmt("hostname: %w", err)
	case ib.GetHostname() != "" && ib.Hostname != bbHost:
		return errors.Fmt("incorrect hostname, want: %s, got: %s", bbHost, ib.Hostname)
	case teeErr(validateAgent(ib.GetAgent()), &err) != nil:
		return errors.Fmt("agent: %w", err)
	case teeErr(validateRequestedDimensions(ib.RequestedDimensions), &err) != nil:
		return errors.Fmt("requested_dimensions: %w", err)
	case teeErr(validateProperties(ib.RequestedProperties), &err) != nil:
		return errors.Fmt("requested_properties: %w", err)
	}
	for _, host := range ib.GetKnownPublicGerritHosts() {
		if err = validateHostName(host); err != nil {
			return errors.Fmt("known_public_gerrit_hosts: %w", err)
		}
	}
	return nil
}

func convertSwarmingCaches(swarmingCaches []*pb.BuildInfra_Swarming_CacheEntry) []*pb.CacheEntry {
	caches := make([]*pb.CacheEntry, len(swarmingCaches))
	for i, c := range swarmingCaches {
		caches[i] = &pb.CacheEntry{
			Name:             c.Name,
			Path:             c.Path,
			WaitForWarmCache: c.WaitForWarmCache,
			EnvVar:           c.EnvVar,
		}
	}
	return caches
}

func validateCaches(caches []*pb.CacheEntry) error {
	names := stringset.New(len(caches))
	paths := stringset.New(len(caches))
	for i, cache := range caches {
		switch {
		case cache.Name == "":
			return errors.New(fmt.Sprintf("%dth cache: name unspecified", i))
		case len(cache.Name) > 128:
			return errors.New(fmt.Sprintf("%dth cache: name too long (limit is 128)", i))
		case !names.Add(cache.Name):
			return errors.New(fmt.Sprintf("duplicated cache name: %s", cache.Name))
		case cache.Path == "":
			return errors.New(fmt.Sprintf("%dth cache: path unspecified", i))
		case strings.Contains(cache.Path, "\\"):
			return errors.New(fmt.Sprintf("%dth cache: path must use POSIX format", i))
		case !paths.Add(cache.Path):
			return errors.New(fmt.Sprintf("duplicated cache path: %s", cache.Path))
		case cache.WaitForWarmCache.AsDuration()%(60*time.Second) != 0:
			return errors.New(fmt.Sprintf("%dth cache: wait_for_warm_cache must be multiples of 60 seconds.", i))
		}
	}
	return nil
}

func validateDimensionKey(k string) error {
	if err := validateKeyLength(k); err != nil {
		return err
	}
	if !dimensionKeyRe.MatchString(k) {
		return errors.Fmt("the key should match %s", dimensionKeyRe)
	}
	return nil
}

func validateDimensionValue(v string) error {
	if v == "" {
		return errors.New("the value cannot be empty")
	}
	return validateTagValue(v)
}

// validateDimension validates the task dimension.
func validateDimension(dim *pb.RequestedDimension, allowEmptyValue bool) error {
	var err error
	switch {
	case teeErr(validateExpirationDuration(dim.GetExpiration()), &err) != nil:
		return errors.Fmt("expiration: %w", err)
	case teeErr(validateDimensionKey(dim.GetKey()), &err) != nil:
		return err
	case allowEmptyValue && dim.GetValue() == "":
		return nil
	case teeErr(validateDimensionValue(dim.GetValue()), &err) != nil:
		return err
	default:
		return nil
	}
}

// validateDimensions validates the task dimensions.
func validateDimensions(dims []*pb.RequestedDimension) error {
	for i, dim := range dims {
		switch err := validateDimension(dim, false); {
		case err != nil:
			return errors.Fmt("[%d]: %w", i, err)
		case dim.Value == "":
			return errors.Fmt("[%d]: value must be specified", i)
		}
	}
	return nil
}

func validateBackendConfig(config *structpb.Struct) error {
	if config == nil {
		return nil
	}

	var priority float64
	if p := config.GetFields()["priority"]; p != nil {
		if _, ok := p.GetKind().(*structpb.Value_NumberValue); !ok {
			return errors.New("priority must be a number")
		}
		priority = p.GetNumberValue()
	}
	// Currently apply the same rule as swarming priority rule for backend priority.
	// This may change when we have other backends in the future.
	if priority < 0 || priority > 255 {
		return errors.New("priority must be in [0, 255]")
	}
	return nil
}

func validateInfraBackend(ctx context.Context, ib *pb.BuildInfra_Backend) error {
	if ib == nil {
		return nil
	}

	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return errors.Fmt("error fetching service config: %w", err)
	}

	switch {
	case teeErr(config.ValidateTaskBackendTarget(globalCfg, ib.GetTask().GetId().GetTarget()), &err) != nil:
		return err
	case teeErr(validateBackendConfig(ib.GetConfig()), &err) != nil:
		return errors.Fmt("config: %w", err)
	case teeErr(validateDimensions(ib.GetTaskDimensions()), &err) != nil:
		return errors.Fmt("task_dimensions: %w", err)
	case teeErr(validateCaches(ib.GetCaches()), &err) != nil:
		return errors.Fmt("caches: %w", err)
	default:
		return nil
	}
}

func validateInfraSwarming(is *pb.BuildInfra_Swarming) error {
	var err error
	if is == nil {
		return nil
	}
	switch {
	case teeErr(validateHostName(is.GetHostname()), &err) != nil:
		return errors.Fmt("hostname: %w", err)
	case is.GetPriority() < 0 || is.GetPriority() > 255:
		return errors.New("priority must be in [0, 255]")
	case teeErr(validateDimensions(is.GetTaskDimensions()), &err) != nil:
		return errors.Fmt("task_dimensions: %w", err)
	case teeErr(validateCaches(convertSwarmingCaches(is.GetCaches())), &err) != nil:
		return errors.Fmt("caches: %w", err)
	default:
		return nil
	}
}

func validateInfraLogDog(il *pb.BuildInfra_LogDog) error {
	var err error
	switch {
	case teeErr(validateHostName(il.GetHostname()), &err) != nil:
		return errors.Fmt("hostname: %w", err)
	default:
		return nil
	}
}

func validateInfraResultDB(irdb *pb.BuildInfra_ResultDB) error {
	var err error
	switch {
	case irdb == nil:
		return nil
	case teeErr(validateHostName(irdb.GetHostname()), &err) != nil:
		return errors.Fmt("hostname: %w", err)
	default:
		return nil
	}
}

func validateInfra(ctx context.Context, infra *pb.BuildInfra) error {
	var err error
	switch {
	case infra.GetBackend() == nil && infra.GetSwarming() == nil:
		return errors.New("backend or swarming is needed in build infra")
	case infra.GetBackend() != nil && infra.GetSwarming() != nil:
		return errors.New("can only have one of backend or swarming in build infra. both were provided")
	case teeErr(validateInfraBackend(ctx, infra.GetBackend()), &err) != nil:
		return errors.Fmt("backend: %w", err)
	case teeErr(validateInfraSwarming(infra.GetSwarming()), &err) != nil:
		return errors.Fmt("swarming: %w", err)
	case teeErr(validateInfraBuildbucket(ctx, infra.GetBuildbucket()), &err) != nil:
		return errors.Fmt("buildbucket: %w", err)
	case teeErr(validateInfraLogDog(infra.GetLogdog()), &err) != nil:
		return errors.Fmt("logdog: %w", err)
	case teeErr(validateInfraResultDB(infra.GetResultdb()), &err) != nil:
		return errors.Fmt("resultdb: %w", err)
	default:
		return nil
	}
}

func validateInput(wellKnownExperiments stringset.Set, in *pb.Build_Input) error {
	var err error
	switch {
	case teeErr(validateGerritChanges(in.GerritChanges), &err) != nil:
		return errors.Fmt("gerrit_changes: %w", err)
	case in.GetGitilesCommit() != nil && teeErr(validateCommitWithRef(in.GitilesCommit), &err) != nil:
		return errors.Fmt("gitiles_commit: %w", err)
	case in.Properties != nil && teeErr(validateProperties(in.Properties), &err) != nil:
		return errors.Fmt("properties: %w", err)
	}
	for _, expName := range in.Experiments {
		if err := config.ValidateExperimentName(expName, wellKnownExperiments); err != nil {
			return errors.Fmt("experiment %q: %w", expName, err)
		}
	}
	return nil
}

func validateExe(exe *pb.Executable, agent *pb.BuildInfra_Buildbucket_Agent) error {
	var err error
	switch {
	case exe.GetCipdPackage() == "":
		return nil
	case teeErr(validateCipdPackage(exe.CipdPackage, false), &err) != nil:
		return errors.Fmt("cipd_package: %w", err)
	case exe.GetCipdVersion() != "" && teeErr(cipdCommon.ValidateInstanceVersion(exe.CipdVersion), &err) != nil:
		return errors.Fmt("cipd_version: %w", err)
	}

	// Validate exe matches with agent.
	var payloadPath string
	for dir, purpose := range agent.GetPurposes() {
		if purpose == pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD {
			payloadPath = dir
			break
		}
	}
	if payloadPath == "" {
		return nil
	}

	if pkgs, ok := agent.GetInput().GetData()[payloadPath]; ok {
		cipdPkgs := pkgs.GetCipd()
		if cipdPkgs == nil {
			return errors.New("not match build.infra.buildbucket.agent")
		}

		packageMatches := false
		for _, spec := range cipdPkgs.Specs {
			if spec.Package != exe.CipdPackage {
				continue
			}
			packageMatches = true
			if spec.Version != exe.CipdVersion {
				return errors.New("cipd_version does not match build.infra.buildbucket.agent")
			}
			break
		}
		if !packageMatches {
			return errors.New("cipd_package does not match build.infra.buildbucket.agent")
		}
	}
	return nil
}

func validateBuild(ctx context.Context, wellKnownExperiments stringset.Set, b *pb.Build) error {
	var err error
	switch {
	case teeErr(protoutil.ValidateRequiredBuilderID(b.Builder), &err) != nil:
		return errors.Fmt("builder: %w", err)
	case teeErr(validateExe(b.Exe, b.GetInfra().GetBuildbucket().GetAgent()), &err) != nil:
		return errors.Fmt("exe: %w", err)
	case teeErr(validateInput(wellKnownExperiments, b.Input), &err) != nil:
		return errors.Fmt("input: %w", err)
	case teeErr(validateInfra(ctx, b.Infra), &err) != nil:
		return errors.Fmt("infra: %w", err)
	case teeErr(validateBucketConstraints(ctx, b), &err) != nil:
		return err
	case teeErr(validateTags(b.Tags, TagNew), &err) != nil:
		return errors.Fmt("tags: %w", err)
	default:
		return nil
	}
}

var cbrWalker = protowalk.NewWalker[*pb.CreateBuildRequest](
	protowalk.DeprecatedProcessor{},
	protowalk.OutputOnlyProcessor{},
	protowalk.RequiredProcessor{},
	CreateBuildChecker{},
)

func validateCreateBuildRequest(ctx context.Context, wellKnownExperiments stringset.Set, req *pb.CreateBuildRequest) (*model.BuildMask, error) {
	if procRes := cbrWalker.Execute(req); !procRes.Empty() {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			logging.Infof(ctx, strings.Join(resStrs, ". "))
		}
		if err := procRes.Err(); err != nil {
			return nil, err
		}
	}

	if err := validateBuild(ctx, wellKnownExperiments, req.GetBuild()); err != nil {
		return nil, errors.Fmt("build: %w", err)
	}

	if strings.Contains(req.GetRequestId(), "/") {
		return nil, errors.New("request_id cannot contain '/'")
	}

	m, err := model.NewBuildMask("", nil, req.Mask)
	if err != nil {
		return nil, errors.Fmt("invalid mask: %w", err)
	}

	return m, nil
}

// buildToCreate specifies a build to store and launch along with some related
// parameters.
type buildToCreate struct {
	// A valid build to be save in datastore and updated in-place.
	build *model.Build
	// The request ID for deduplication, if any.
	requestID string
	// ResultDB options for this build.
	resultDB resultdb.CreateOptions
}

// createBuilds saves the builds to datastore and triggers TQ tasks needed to
// actually start running these builds.
//
// Builds of builders that are in the given `bldrsMCB` set will be launched
// via a mechanism that enforces `max_concurrent_builds` limit.
//
// Updates successfully created builds in-place. Always returns exactly
// len(builds) errors.
func createBuilds(ctx context.Context, builds []*buildToCreate, bldrsMCB stringset.Set) errors.MultiError {
	now := clock.Now(ctx).UTC()
	user := auth.CurrentIdentity(ctx)
	appID := info.AppID(ctx) // e.g. cr-buildbucket
	ids := buildid.NewBuildIDs(ctx, now, len(builds))
	nums := make([]*model.Build, 0, len(builds))
	var idxMapNums []int

	for i, b := range builds {
		b.build.ID = ids[i]
		b.build.CreatedBy = user
		b.build.CreateTime = now

		// Set proto field values which can only be determined at creation-time.
		b.build.Proto.CreatedBy = string(user)
		b.build.Proto.CreateTime = timestamppb.New(now)
		b.build.Proto.Id = ids[i]
		if b.build.Proto.Infra.Buildbucket.Hostname == "" {
			b.build.Proto.Infra.Buildbucket.Hostname = fmt.Sprintf("%s.appspot.com", appID)
		}
		b.build.Proto.Infra.Logdog.Prefix = fmt.Sprintf("buildbucket/%s/%d", appID, b.build.Proto.Id)
		protoutil.SetStatus(now, b.build.Proto, pb.Status_SCHEDULED)

		if b.build.Proto.GetInfra().GetBuildbucket().GetBuildNumber() {
			idxMapNums = append(idxMapNums, i)
			nums = append(nums, b.build)
		}
	}

	// Attempt to generate build numbers for builds that requested them.
	merr := make(errors.MultiError, len(builds))
	if err := generateBuildNumbers(ctx, nums); err != nil {
		merr = mergeErrs(merr, err.(errors.MultiError), "error generating build numbers", func(idx int) int { return idxMapNums[idx] })
	}

	// Get still pending builds along with their indexes in `builds`.
	pendingBuilds := make([]*model.Build, 0, len(builds))
	filteredRDBOpts := make([]resultdb.CreateOptions, 0, len(builds))
	idxMapPending := make([]int, 0, len(builds))
	for idx, build := range builds {
		if merr[idx] != nil {
			continue
		}
		pendingBuilds = append(pendingBuilds, build.build)
		filteredRDBOpts = append(filteredRDBOpts, build.resultDB)
		idxMapPending = append(idxMapPending, idx)
	}

	err := parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error { return model.UpdateBuilderStat(ctx, pendingBuilds, now) }
		work <- func() error { return resultdb.CreateInvocations(ctx, pendingBuilds, filteredRDBOpts) }
		work <- func() error { return search.UpdateTagIndex(ctx, pendingBuilds) }
		work <- func() error {
			// Evaluate the builds for custom builder metrics.
			// The builds have not been saved in datastore, so nothing to load as
			// build details.
			for _, bld := range pendingBuilds {
				if err := model.EvaluateBuildForCustomBuilderMetrics(ctx, bld, bld.Proto, false); err != nil {
					logging.Errorf(ctx, "failed to evaluate build for custom builder metrics: %s", err)
				}
			}
			return nil
		}
	})
	if err != nil {
		errs := err.(errors.MultiError)
		for _, e := range errs {
			if me, ok := e.(errors.MultiError); ok {
				merr = mergeErrs(merr, me, "", func(idx int) int { return idxMapPending[idx] })
			} else {
				// Fatal global error. Update all pending builds with this error.
				for i, cur := range merr {
					if cur == nil {
						merr[i] = e
					}
				}
				return merr
			}
		}
	}

	// Get still pending builds along with their indexes in `builds`.
	pendingBuilds = pendingBuilds[:0]
	idxMapPending = idxMapPending[:0]
	for idx, build := range builds {
		if merr[idx] != nil {
			continue
		}
		pendingBuilds = append(pendingBuilds, build.build)
		idxMapPending = append(idxMapPending, idx)
	}

	// This parallel work isn't combined with the above parallel work to ensure
	// build entities and Swarming (or Backend) task creation tasks are only
	// created if everything else has succeeded (since everything can't be done
	// in one transaction).
	_ = parallel.WorkPool(min(64, len(pendingBuilds)), func(work chan<- func() error) {
		for i, b := range pendingBuilds {
			reqID := builds[idxMapPending[i]].requestID

			work <- func() error {
				bldrID := b.Proto.Builder
				bs := &model.BuildStatus{
					Build:        datastore.KeyForObj(ctx, b),
					Status:       pb.Status_SCHEDULED,
					BuildAddress: fmt.Sprintf("%s/%s/%s/b%d", bldrID.Project, bldrID.Bucket, bldrID.Builder, b.ID),
				}
				if b.Proto.Number > 0 {
					bs.BuildAddress = fmt.Sprintf("%s/%s/%s/%d", bldrID.Project, bldrID.Bucket, bldrID.Builder, b.Proto.Number)
				}
				toPut := []any{
					b,
					bs,
					&model.BuildInfra{
						Build: datastore.KeyForObj(ctx, b),
						Proto: b.Proto.Infra,
					},
					&model.BuildInputProperties{
						Build: datastore.KeyForObj(ctx, b),
						Proto: b.Proto.Input.Properties,
					},
				}
				r := model.NewRequestID(ctx, b.ID, now, reqID)

				// Write the entities and trigger a task queue task to create the Swarming task.
				err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					// Deduplicate by request ID.
					if reqID != "" {
						switch err := datastore.Get(ctx, r); {
						case err == datastore.ErrNoSuchEntity:
							toPut = append(toPut, r)
						case err != nil:
							return errors.Fmt("failed to deduplicate request ID: %d: %w", b.ID, err)
						default:
							b.ID = r.BuildID
							if err := datastore.Get(ctx, b); err != nil {
								return errors.Fmt("failed to fetch deduplicated build: %d: %w", b.ID, err)
							}
							return nil
						}
					}

					// Request was not a duplicate.
					switch err := datastore.Get(ctx, &model.Build{ID: b.ID}); {
					case err == nil:
						return appstatus.Errorf(codes.AlreadyExists, "build already exists: %d", b.ID)
					case err != datastore.ErrNoSuchEntity:
						return errors.Fmt("failed to fetch build: %d: %w", b.ID, err)
					}

					// Drop the infra, input.properties when storing into Build entity, as
					// they are stored in separate datastore entities.
					infra := b.Proto.Infra
					inProp := b.Proto.Input.Properties
					b.Proto.Infra = nil
					b.Proto.Input.Properties = nil
					defer func() {
						b.Proto.Infra = infra
						b.Proto.Input.Properties = inProp
					}()
					if err := datastore.Put(ctx, toPut...); err != nil {
						return errors.Fmt("failed to store build: %d: %w", b.ID, err)
					}

					switch {
					case bldrsMCB.Has(protoutil.FormatBuilderID(bldrID)):
						// max_concurrent_builds feature is enabled for this builder.
						if err := tasks.CreatePushPendingBuildTask(ctx, &taskdefs.PushPendingBuildTask{
							BuildId:   b.ID,
							BuilderId: bldrID,
						}); err != nil {
							return errors.Fmt("failed to enqueue PushPendingBuildTask: %w", err)
						}
					case infra.GetBackend() != nil:
						// If a backend is set, create a backend task.
						if err := tasks.CreateBackendBuildTask(ctx, &taskdefs.CreateBackendBuildTask{
							BuildId:   b.ID,
							RequestId: uuid.New().String(),
						}); err != nil {
							return errors.Fmt("failed to enqueue CreateBackendTask: %w", err)
						}
					case infra.GetSwarming().GetHostname() == "":
						return errors.New("failed to create build with missing backend info and swarming host")
					default:
						// Otherwise, create a swarming task.
						if err := tasks.CreateSwarmingBuildTask(ctx, &taskdefs.CreateSwarmingBuildTask{
							BuildId: b.ID,
						}); err != nil {
							return errors.Fmt("failed to enqueue CreateSwarmingBuildTask: %d: %w", b.ID, err)
						}
					}

					if err := tasks.NotifyPubSub(ctx, b); err != nil {
						// Don't fail the entire creation. Just log the error since the
						// status notification for unspecified -> scheduled is a
						// nice-to-have not a must-to-have.
						logging.Warningf(ctx, "failed to enqueue the notification when Build(%d) is scheduled: %s", b.ID, err)
					}
					return nil
				}, nil)

				// Record any error happened in the above transaction.
				if err != nil {
					merr[idxMapPending[i]] = err
				} else {
					metrics.BuildCreated(ctx, b)
				}
				return nil
			}
		}
	})

	return merr
}

// generateBuildNumbers mutates the given builds, setting build numbers and
// build address tags.
//
// It would return a MultiError (if any) where len(MultiError) equals to
// len(reqs).
func generateBuildNumbers(ctx context.Context, builds []*model.Build) error {
	merr := make(errors.MultiError, len(builds))
	seq := make(map[string][]*model.Build)
	idxMap := make(map[string][]int) // BuilderID -> a list of index
	for i, b := range builds {
		name := protoutil.FormatBuilderID(b.Proto.Builder)
		seq[name] = append(seq[name], b)
		idxMap[name] = append(idxMap[name], i)
	}
	_ = parallel.WorkPool(min(64, len(builds)), func(work chan<- func() error) {
		for name, blds := range seq {
			work <- func() error {
				n, err := model.GenerateSequenceNumbers(ctx, name, len(blds))
				if err != nil {
					for _, idx := range idxMap[name] {
						merr[idx] = err
					}
					return nil
				}
				for i, b := range blds {
					b.Proto.Number = n + int32(i)
					addr := fmt.Sprintf("build_address:luci.%s.%s/%s/%d", b.Proto.Builder.Project, b.Proto.Builder.Bucket, b.Proto.Builder.Builder, b.Proto.Number)
					b.Tags = append(b.Tags, addr)
					sort.Strings(b.Tags)
				}
				return nil
			}
		}
	})

	if merr.First() == nil {
		return nil
	}
	return merr.AsError()
}

// CreateBuild handles a request to create a build. Implements pb.BuildsServer.
func (*Builds) CreateBuild(ctx context.Context, req *pb.CreateBuildRequest) (*pb.Build, error) {
	if err := perm.HasInBucket(ctx, bbperms.BuildsCreate, req.Build.Builder.Project, req.Build.Builder.Bucket); err != nil {
		return nil, err
	}

	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return nil, errors.Fmt("error fetching service config: %w", err)
	}
	wellKnownExperiments := protoutil.WellKnownExperiments(globalCfg)

	m, err := validateCreateBuildRequest(ctx, wellKnownExperiments, req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	bld := &model.Build{
		Proto:  req.Build,
		IsLuci: true,
	}

	// Update ancestors info.
	p := validateParentViaToken(ctx)
	if p.err != nil {
		return nil, errors.Fmt("build parent: %w", p.err)
	}

	if len(p.ancestors) > 0 {
		bld.Proto.AncestorIds = p.ancestors
	}

	resultdbOpts := resultdb.CreateOptions{
		IsExportRoot: p.bld == nil,
	}

	setExperimentsFromProto(bld)
	// Tags are stored in the outer struct (see model/build.go).
	tagMap := protoutil.StringPairMap(bld.Proto.Tags)
	if p.pRunID != "" {
		tagMap.Add("parent_task_id", p.pRunID)
	}
	tags := tagMap.Format()
	tags = stringset.NewFromSlice(tags...).ToSlice() // Deduplicate tags.
	sort.Strings(tags)
	bld.Tags = tags

	merr := createBuilds(ctx, []*buildToCreate{
		{
			build:     bld,
			resultDB:  resultdbOpts,
			requestID: req.RequestId,
		},
	}, nil)
	if merr[0] != nil {
		return nil, errors.Fmt("error creating build: %w", merr[0])
	}

	return bld.ToProto(ctx, m, nil)
}

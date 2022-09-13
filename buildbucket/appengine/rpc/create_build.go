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
	"strings"
	"time"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	cipdCommon "go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/bbperms"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

type CreateBuildChecker struct{}

var _ protowalk.FieldProcessor = (*CreateBuildChecker)(nil)

func (*CreateBuildChecker) Process(field protoreflect.FieldDescriptor, msg protoreflect.Message) (data protowalk.ResultData, applied bool) {
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

func init() {
	protowalk.RegisterFieldProcessor(&CreateBuildChecker{}, func(field protoreflect.FieldDescriptor) protowalk.ProcessAttr {
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
	})
}

func validateBucketConstraints(ctx context.Context, b *pb.Build) error {
	bck := &model.Bucket{
		Parent: model.ProjectKey(ctx, b.Builder.Project),
		ID:     b.Builder.Bucket,
	}
	bckStr := fmt.Sprintf("%s:%s", b.Builder.Project, b.Builder.Bucket)
	if err := datastore.Get(ctx, bck); err != nil {
		return errors.Annotate(err, "failed to fetch bucket config %s", bckStr).Err()
	}

	constraints := bck.Proto.GetConstraints()
	if constraints == nil {
		return errors.Reason("constraints for %s not found", bckStr).Err()
	}

	allowedPools := stringset.NewFromSlice(constraints.GetPools()...)
	allowedSAs := stringset.NewFromSlice(constraints.GetServiceAccounts()...)
	poolAllowed := false
	var pool string
	for _, dim := range b.GetInfra().GetSwarming().GetTaskDimensions() {
		if dim.Key != "pool" {
			continue
		}
		pool = dim.Value
		if allowedPools.Has(dim.Value) {
			poolAllowed = true
			break
		}
	}
	if !poolAllowed {
		return errors.Reason("build.infra.swarming.dimension['pool'] %s: not allowed", pool).Err()
	}

	sa := b.GetInfra().GetSwarming().GetTaskServiceAccount()
	if sa == "" || !allowedSAs.Has(sa) {
		return errors.Reason("build.infra.swarming.task_service_account %s: not allowed", sa).Err()
	}
	return nil
}

func validateHostName(host string) error {
	if strings.Contains(host, "://") {
		return errors.Reason(`must not contain "://"`).Err()
	}
	return nil
}

func validateAgentInput(in *pb.BuildInfra_Buildbucket_Agent_Input) error {
	for path, ref := range in.GetData() {
		for i, spec := range ref.GetCipd().GetSpecs() {
			if err := cipdCommon.ValidatePackageName(spec.GetPackage()); err != nil {
				return errors.Annotate(err, "[%s]: [%d]: cipd.package", path, i).Err()
			}
			if err := cipdCommon.ValidateInstanceVersion(spec.GetVersion()); err != nil {
				return errors.Annotate(err, "[%s]: [%d]: cipd.version", path, i).Err()
			}
		}
	}
	return nil
}

func validateAgentSource(src *pb.BuildInfra_Buildbucket_Agent_Source) error {
	cipd := src.GetCipd()
	if !strings.HasSuffix(cipd.GetPackage(), "/${platform}") {
		return errors.Reason("cipd.package: must end with '/${platform}'").Err()
	}
	if err := cipdCommon.ValidateInstanceVersion(cipd.GetVersion()); err != nil {
		return errors.Annotate(err, "cipd.version").Err()
	}
	return nil
}

func validateAgentPurposes(purposes map[string]pb.BuildInfra_Buildbucket_Agent_Purpose, in *pb.BuildInfra_Buildbucket_Agent_Input) error {
	if len(purposes) == 0 {
		return nil
	}

	for path := range purposes {
		if _, ok := in.GetData()[path]; !ok {
			return errors.Reason("Invalid path %s - not in input dataRef", path).Err()
		}
	}
	return nil
}

func validateAgent(agent *pb.BuildInfra_Buildbucket_Agent) error {
	var err error
	switch {
	case teeErr(validateAgentInput(agent.GetInput()), &err) != nil:
		return errors.Annotate(err, "input").Err()
	case teeErr(validateAgentSource(agent.GetSource()), &err) != nil:
		return errors.Annotate(err, "source").Err()
	case teeErr(validateAgentPurposes(agent.GetPurposes(), agent.GetInput()), &err) != nil:
		return errors.Annotate(err, "purposes").Err()
	default:
		return nil
	}
}

func validateInfraBuildbucket(ib *pb.BuildInfra_Buildbucket) error {
	var err error
	switch {
	case teeErr(validateHostName(ib.GetHostname()), &err) != nil:
		return errors.Annotate(err, "hostname").Err()
	case teeErr(validateAgent(ib.GetAgent()), &err) != nil:
		return errors.Annotate(err, "agent").Err()
	case teeErr(validateRequestedDimensions(ib.RequestedDimensions), &err) != nil:
		return errors.Annotate(err, "requested_dimensions").Err()
	case teeErr(validateProperties(ib.RequestedProperties), &err) != nil:
		return errors.Annotate(err, "requested_properties").Err()
	}
	for _, host := range ib.GetKnownPublicGerritHosts() {
		if err = validateHostName(host); err != nil {
			return errors.Annotate(err, "known_public_gerrit_hosts").Err()
		}
	}
	return nil
}

func validateCaches(caches []*pb.BuildInfra_Swarming_CacheEntry) error {
	names := stringset.New(len(caches))
	pathes := stringset.New(len(caches))
	for i, cache := range caches {
		switch {
		case cache.Name == "":
			return errors.Reason(fmt.Sprintf("%dth cache: name unspecified", i)).Err()
		case len(cache.Name) > 128:
			return errors.Reason(fmt.Sprintf("%dth cache: name too long (limit is 128)", i)).Err()
		case !names.Add(cache.Name):
			return errors.Reason(fmt.Sprintf("duplicated cache name: %s", cache.Name)).Err()
		case cache.Path == "":
			return errors.Reason(fmt.Sprintf("%dth cache: path unspecified", i)).Err()
		case strings.Contains(cache.Path, "\\"):
			return errors.Reason(fmt.Sprintf("%dth cache: path must use POSIX format", i)).Err()
		case !pathes.Add(cache.Path):
			return errors.Reason(fmt.Sprintf("duplicated cache path: %s", cache.Path)).Err()
		case cache.WaitForWarmCache.AsDuration()%(60*time.Second) != 0:
			return errors.Reason(fmt.Sprintf("%dth cache: wait_for_warm_cache must be multiples of 60 seconds.", i)).Err()
		}
	}
	return nil
}

// validateDimensions validates the task dimension.
func validateDimension(dim *pb.RequestedDimension) error {
	var err error
	switch {
	case teeErr(validateExpirationDuration(dim.GetExpiration()), &err) != nil:
		return errors.Annotate(err, "expiration").Err()
	case dim.GetKey() == "":
		return errors.Reason("key must be specified").Err()
	case dim.Value == "":
		return errors.Reason("value must be specified").Err()
	default:
		return nil
	}
}

// validateDimensions validates the task dimensions.
func validateDimensions(dims []*pb.RequestedDimension) error {
	for i, dim := range dims {
		if err := validateDimension(dim); err != nil {
			return errors.Annotate(err, "[%d]", i).Err()
		}
	}
	return nil
}

func validateInfraSwarming(is *pb.BuildInfra_Swarming) error {
	var err error
	switch {
	case teeErr(validateHostName(is.GetHostname()), &err) != nil:
		return errors.Annotate(err, "hostname").Err()
	case is.GetPriority() < 0 || is.GetPriority() > 255:
		return errors.Reason("priority must be in [0, 255]").Err()
	case teeErr(validateDimensions(is.GetTaskDimensions()), &err) != nil:
		return errors.Annotate(err, "task_dimensions").Err()
	case teeErr(validateCaches(is.GetCaches()), &err) != nil:
		return errors.Annotate(err, "caches").Err()
	default:
		return nil
	}
}

func validateInfraLogDog(il *pb.BuildInfra_LogDog) error {
	var err error
	switch {
	case teeErr(validateHostName(il.GetHostname()), &err) != nil:
		return errors.Annotate(err, "hostname").Err()
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
		return errors.Annotate(err, "hostname").Err()
	default:
		return nil
	}
}

func validateInfra(infra *pb.BuildInfra) error {
	var err error
	switch {
	case teeErr(validateInfraBuildbucket(infra.GetBuildbucket()), &err) != nil:
		return errors.Annotate(err, "buildbucket").Err()
	case teeErr(validateInfraSwarming(infra.GetSwarming()), &err) != nil:
		return errors.Annotate(err, "swarming").Err()
	case teeErr(validateInfraLogDog(infra.GetLogdog()), &err) != nil:
		return errors.Annotate(err, "logdog").Err()
	case teeErr(validateInfraResultDB(infra.GetResultdb()), &err) != nil:
		return errors.Annotate(err, "resultdb").Err()
	case infra.GetBackend() != nil:
		return errors.Reason("backend: should not be specified").Err()
	default:
		return nil
	}
}

func validateInput(wellKnownExperiments stringset.Set, in *pb.Build_Input) error {
	var err error
	switch {
	case teeErr(validateGerritChanges(in.GerritChanges), &err) != nil:
		return errors.Annotate(err, "gerrit_changes").Err()
	case in.GetGitilesCommit() != nil && teeErr(validateCommitWithRef(in.GitilesCommit), &err) != nil:
		return errors.Annotate(err, "gitiles_commit").Err()
	case in.Properties != nil && teeErr(validateProperties(in.Properties), &err) != nil:
		return errors.Annotate(err, "properties").Err()
	}
	for _, expName := range in.Experiments {
		if err := config.ValidateExperimentName(expName, wellKnownExperiments); err != nil {
			return errors.Annotate(err, "experiment %q", expName).Err()
		}
	}
	return nil
}

func validateExe(exe *pb.Executable, agent *pb.BuildInfra_Buildbucket_Agent) error {
	var err error
	switch {
	case exe.GetCipdPackage() != "" && teeErr(cipdCommon.ValidatePackageName(exe.CipdPackage), &err) != nil:
		return errors.Annotate(err, "cipd_package").Err()
	case exe.GetCipdVersion() != "" && teeErr(cipdCommon.ValidateInstanceVersion(exe.CipdVersion), &err) != nil:
		return errors.Annotate(err, "cipd_version").Err()
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
			return errors.Reason("not match build.infra.buildbucket.agent").Err()
		}

		packageMatches := false
		for _, spec := range cipdPkgs.Specs {
			if spec.Package != exe.CipdPackage {
				continue
			}
			packageMatches = true
			if spec.Version != exe.CipdVersion {
				return errors.Reason("cipd_version does not match build.infra.buildbucket.agent").Err()
			}
			break
		}
		if !packageMatches {
			return errors.Reason("cipd_package does not match build.infra.buildbucket.agent").Err()
		}
	}
	return nil
}

func validateBuild(ctx context.Context, wellKnownExperiments stringset.Set, b *pb.Build) error {
	var err error
	switch {
	case teeErr(protoutil.ValidateRequiredBuilderID(b.Builder), &err) != nil:
		return errors.Annotate(err, "builder").Err()
	case teeErr(validateExe(b.Exe, b.GetInfra().GetBuildbucket().GetAgent()), &err) != nil:
		return errors.Annotate(err, "exe").Err()
	case teeErr(validateInput(wellKnownExperiments, b.Input), &err) != nil:
		return errors.Annotate(err, "input").Err()
	case teeErr(validateInfra(b.Infra), &err) != nil:
		return errors.Annotate(err, "infra").Err()
	case teeErr(validateBucketConstraints(ctx, b), &err) != nil:
		return err
	case teeErr(validateTags(b.Tags, TagNew), &err) != nil:
		return errors.Annotate(err, "tags").Err()
	default:
		return nil
	}
}

func validateCreateBuildRequest(ctx context.Context, wellKnownExperiments stringset.Set, req *pb.CreateBuildRequest) (*model.BuildMask, error) {
	if procRes := protowalk.Fields(req, &protowalk.DeprecatedProcessor{}, &protowalk.OutputOnlyProcessor{}, &protowalk.RequiredProcessor{}, &CreateBuildChecker{}); procRes != nil {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			logging.Infof(ctx, strings.Join(resStrs, ". "))
		}
		if err := procRes.Err(); err != nil {
			return nil, err
		}
	}

	if err := validateBuild(ctx, wellKnownExperiments, req.GetBuild()); err != nil {
		return nil, errors.Annotate(err, "build").Err()
	}

	if strings.Contains(req.GetRequestId(), "/") {
		return nil, errors.Reason("request_id cannot contain '/'").Err()
	}

	m, err := model.NewBuildMask("", nil, req.Mask)
	if err != nil {
		return nil, errors.Annotate(err, "invalid mask").Err()
	}

	return m, nil
}

// CreateBuild handles a request to schedule a build. Implements pb.BuildsServer.
func (*Builds) CreateBuild(ctx context.Context, req *pb.CreateBuildRequest) (*pb.Build, error) {
	if err := perm.HasInBucket(ctx, bbperms.BuildsCreate, req.Build.Builder.Project, req.Build.Builder.Bucket); err != nil {
		return nil, err
	}

	globalCfg, err := config.GetSettingsCfg(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "error fetching service config").Err()
	}
	wellKnownExperiments := protoutil.WellKnownExperiments(globalCfg)

	_, err = validateCreateBuildRequest(ctx, wellKnownExperiments, req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	return nil, errors.New("not implemented")
}

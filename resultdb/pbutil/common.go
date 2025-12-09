// Copyright 2019 The LUCI Authors.
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

package pbutil

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"golang.org/x/text/unicode/norm"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const MaxSizeInvocationProperties = 16 * 1024 // 16 KB

const MaxSizeTestMetadataProperties = 4 * 1024 // 4 KB

// MaxSizeTestResultProperties is the maximum size of the test result
// properties.
//
// CAVEAT: before increasing the size limit, verify if it will break downstream
// services. Notably the test verdict exports. BigQuery has a 10 MB AppendRows
// request size limit[1]. Each verdict can have a maximum of 100 test results[2]
// as of 2024-04-11.
//
// [1]: https://cloud.google.com/bigquery/quotas#write-api-limits
// [2]: https://chromium.googlesource.com/infra/luci/luci-go/+/e83766e441050596aaaa22dbdaad4228bacf0929/resultdb/internal/testvariants/query.go#49
const MaxSizeTestResultProperties = 8 * 1024 // 8 KB

const MaxInstructionsSize = 1024 * 1024 // 1 MB

const MaxInstructionSize = 10 * 1024 // 10 KB

const MaxDependencyBuildIDSize = 100

const MaxDependencyStepNameSize = 1024

const MaxDependencyStepTagKeySize = 256

const MaxDependencyStepTagValSize = 1024

const MaxInstructionNameSize = 100

const (
	// The maximum length allowed for Android Branch names.
	// Note: The upstream technically does not enforce a limit but we
	// enforce a reasonable limit here so that we have a number we can design with.
	MaxAndroidBranchLength = 255 // bytes

	// The maximum length allowed for Android Build Target names.
	// Note: The upstream technically does not enforce a limit but we
	// enforce a reasonable limit here so that we have a number we can design with.
	MaxAndroidBuildTargetLength = 255 // bytes

	// The maximum length allowed for Android Build IDs.
	MaxAndroidBuildLength = 32 // bytes
)

// The maximum size the requests collection in a batch request, in bytes.
const MaxBatchRequestSize = 10 * 1024 * 1024 // 10 MiB

// maxResourceNameLength is the maximum length of a full resource name.
// Selected as 2000 bytes here as some browsers and load balancers have
// trouble for URLs above 2,083 characters.
const maxResourceNameLength = 2000

var requestIDRe = regexp.MustCompile(`^[[:ascii:]]{0,36}$`)

// Allow hostnames permitted by
// https://www.rfc-editor.org/rfc/rfc1123#page-13. (Note that
// the 255 character limit must be seperately applied.)
var hostnameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]+(\.[a-z0-9-]+)*$`)

// The maximum hostname permitted by
// https://www.rfc-editor.org/rfc/rfc1123#page-13.
const hostnameMaxLength = 255

const propertyTypeNamePattern = `[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+`

var propertyTypeNameRe = regexpf("^%s$", propertyTypeNamePattern)

var sha1Regex = regexp.MustCompile(`^[a-f0-9]{40}$`)

// androidBuildIDRe defines valid Android build identifiers.
// P indicates a Pending build, E an external build, L a local build. If no
// prefix is present, it indicates a submitted build.
var androidBuildIDRe = regexp.MustCompile(`^[ELP]?[1-9][0-9]*$`)

var (
	// Validates enum values are specified in kebab-case as recommended by
	// https://google.aip.dev/126#alternatives.
	producerResourceSystemRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// Limit to a sensible alphabet. This will allow swarming deployment names like
	// "chromium-swarm-dev" as well as known data realms like "prod", "test", "qual".
	producerDataRealmRE = regexp.MustCompile(`^([a-z0-9\-_]+)*$`)
)

const (
	// maxProducerResourceNameLength is the maximum length of a producer resource name.
	maxProducerResourceNameLength      = 1000 // bytes
	maxProducerResourceDataRealmLength = 100  // bytes
	maxProducerResourceSystemLength    = 50   // bytes
)

func regexpf(patternFormat string, subpatterns ...any) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(patternFormat, subpatterns...))
}

// MustTimestampProto converts a time.Time to a *timestamppb.Timestamp and panics
// on failure.
func MustTimestampProto(t time.Time) *timestamppb.Timestamp {
	ts := timestamppb.New(t)
	if err := ts.CheckValid(); err != nil {
		panic(err)
	}
	return ts
}

// MustTimestamp converts a *timestamppb.Timestamp to a time.Time and panics
// on failure.
func MustTimestamp(ts *timestamppb.Timestamp) time.Time {
	if err := ts.CheckValid(); err != nil {
		panic(err)
	}
	t := ts.AsTime()
	return t
}

// ValidateRequestID returns a non-nil error if requestID is invalid.
// Returns nil if requestID is empty.
func ValidateRequestID(requestID string) error {
	if !requestIDRe.MatchString(requestID) {
		return validate.DoesNotMatchReErr(requestIDRe)
	}
	return nil
}

// ValidateAggregationLevel validates an AggregationLevel is one
// of the enum values and is not unspecified.
func ValidateAggregationLevel(level pb.AggregationLevel) error {
	if level == pb.AggregationLevel_AGGREGATION_LEVEL_UNSPECIFIED {
		return validate.Unspecified()
	}
	if _, ok := pb.AggregationLevel_name[int32(level)]; !ok {
		return errors.Fmt("unknown aggregation level %v", level)
	}
	return nil
}

// ValidateBatchRequestCount validates the number of requests in a batch
// request.
func ValidateBatchRequestCount(count int) error {
	const limit = 500
	if count <= 0 {
		return errors.New("must have at least one request")
	}
	if count > limit {
		return errors.Fmt("the number of requests in the batch (%d) exceeds %d", count, limit)
	}
	return nil
}

// ValidateBatchRequestCountAndSize validates the request count and total size of a batch request.
// Sizes are measured using proto.Size.
func ValidateBatchRequestCountAndSize[T proto.Message](requests []T) error {
	if err := ValidateBatchRequestCount(len(requests)); err != nil {
		return err
	}
	totalSize := 0
	for _, r := range requests {
		totalSize += proto.Size(r)
	}
	if totalSize > MaxBatchRequestSize {
		return errors.Fmt("the size of all requests is too large (got %d bytes; maximum is %d bytes)", totalSize, MaxBatchRequestSize)
	}
	return nil
}

// ValidateEnum returns a non-nil error if the value is not among valid values.
func ValidateEnum(value int32, validValues map[int32]string) error {
	if _, ok := validValues[value]; !ok {
		return errors.Fmt("invalid value %d", value)
	}
	return nil
}

// MustDuration converts a *durationpb.Duration to a time.Duration and panics
// on failure.
func MustDuration(du *durationpb.Duration) time.Duration {
	if err := du.CheckValid(); err != nil {
		panic(err)
	}
	d := du.AsDuration()
	return d
}

// MustMarshal marshals a protobuf message and panics on failure.
func MustMarshal(m protoreflect.ProtoMessage) []byte {
	msg, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	return msg
}

// validateProperties returns a non-nil error if properties is invalid.
func validateProperties(properties *structpb.Struct, maxSize int, requiresType bool) error {
	if properties == nil {
		return nil
	}
	size := proto.Size(properties)
	if size > maxSize {
		return errors.Fmt("the size of properties (%d) exceeds the maximum size of %d bytes", size, maxSize)
	}
	if err := validatePropertiesTypeField(properties, requiresType); err != nil {
		return err
	}
	return nil
}

func validatePropertiesTypeField(value *structpb.Struct, required bool) error {
	typeVal, typeExist := value.Fields["@type"]
	// Ensure @type is specified if it is required.
	if !typeExist {
		if required {
			return errors.New(`must have a field "@type"`)
		}
		return nil
	}
	// Ensure the @type, if specified, is valid.
	typeStr := typeVal.GetStringValue()
	slashIndex := strings.LastIndex(typeStr, "/")
	if slashIndex == -1 {
		return errors.Fmt(`"@type" value %q must contain at least one "/" character`, typeStr)
	}
	if _, err := url.Parse(typeStr); err != nil {
		return errors.Fmt(`"@type" value %q: %w`, typeStr, err)
	}
	typeName := typeStr[slashIndex+1:]
	if err := validate.SpecifiedWithRe(propertyTypeNameRe, typeName); err != nil {
		return errors.Fmt(`"@type" type name %q: %w`, typeName, err)
	}
	return nil
}

// ValidateInvocationProperties returns a non-nil error if properties is invalid.
func ValidateInvocationProperties(properties *structpb.Struct) error {
	return validateProperties(properties, MaxSizeInvocationProperties, false)
}

// ValidateTestResultProperties returns a non-nil error if properties is invalid.
func ValidateTestResultProperties(properties *structpb.Struct) error {
	return validateProperties(properties, MaxSizeTestResultProperties, false)
}

// ValidateTestMetadataProperties returns a non-nil error if properties is invalid.
func ValidateTestMetadataProperties(properties *structpb.Struct) error {
	// Type changed to required since June 27, 2025 as no metadata properties were
	// being uploaded yet.
	return validateProperties(properties, MaxSizeTestMetadataProperties, true)
}

// ValidateWorkUnitProperties returns a non-nil error if properties is invalid
// for a work unit.
func ValidateWorkUnitProperties(properties *structpb.Struct) error {
	return validateProperties(properties, MaxSizeInvocationProperties, true)
}

// ValidateWorkUnitProperties returns a non-nil error if properties is invalid
// for a root invocation.
func ValidateRootInvocationProperties(properties *structpb.Struct) error {
	return validateProperties(properties, MaxSizeInvocationProperties, true)
}

// ValidateGitilesCommit validates a gitiles commit.
func ValidateGitilesCommit(commit *pb.GitilesCommit) error {
	switch {
	case commit == nil:
		return errors.New("unspecified")

	case commit.Host == "":
		return errors.New("host: unspecified")
	case len(commit.Host) > 255:
		return errors.New("host: exceeds 255 characters")
	case !hostnameRE.MatchString(commit.Host):
		return errors.Fmt("host: does not match %q", hostnameRE)

	case commit.Project == "":
		return errors.New("project: unspecified")
	case len(commit.Project) > hostnameMaxLength:
		return errors.Fmt("project: exceeds %v characters", hostnameMaxLength)

	case commit.Ref == "":
		return errors.New("ref: unspecified")

	// The 255 character ref limit is arbitrary and not based on a known
	// restriction in Git. It exists simply because there should be a limit
	// to protect downstream clients.
	case len(commit.Ref) > 255:
		return errors.New("ref: exceeds 255 characters")
	case !strings.HasPrefix(commit.Ref, "refs/"):
		return errors.New("ref: does not match refs/.*")

	case commit.CommitHash == "":
		return errors.New("commit_hash: unspecified")
	case !sha1Regex.MatchString(commit.CommitHash):
		return errors.Fmt("commit_hash: does not match %q", sha1Regex)

	case commit.Position == 0:
		return errors.New("position: unspecified")
	case commit.Position < 0:
		return errors.New("position: cannot be negative")
	}
	return nil
}

// ValidateGerritChange validates a gerrit change.
func ValidateGerritChange(change *pb.GerritChange) error {
	switch {
	case change == nil:
		return errors.New("unspecified")

	case change.Host == "":
		return errors.New("host: unspecified")
	case len(change.Host) > hostnameMaxLength:
		return errors.Fmt("host: exceeds %v characters", hostnameMaxLength)
	case !hostnameRE.MatchString(change.Host):
		return errors.Fmt("host: does not match %q", hostnameRE)

	case change.Project == "":
		return errors.New("project: unspecified")
	// The 255 character project limit is arbitrary and not based on a known
	// restriction in Gerrit. It exists simply because there should be a limit
	// to protect downstream clients.
	case len(change.Project) > 255:
		return errors.New("project: exceeds 255 characters")

	case change.Change == 0:
		return errors.New("change: unspecified")
	case change.Change < 0:
		return errors.New("change: cannot be negative")

	case change.Patchset == 0:
		return errors.New("patchset: unspecified")
	case change.Patchset < 0:
		return errors.New("patchset: cannot be negative")
	default:
		return nil
	}
}

// ValidateSubmittedAndroidBuild validates a submitted Android build reference.
func ValidateSubmittedAndroidBuild(buildID *pb.SubmittedAndroidBuild) error {
	if buildID == nil {
		return errors.New("unspecified")
	}
	if buildID.DataRealm == "" {
		return errors.New("data_realm: unspecified")
	}
	// Support a few common data realm values from go/data-realm.
	if buildID.DataRealm != "prod" && buildID.DataRealm != "test" {
		return errors.Fmt("data_realm: unknown data realm %q", buildID.DataRealm)
	}
	if buildID.Branch == "" {
		return errors.New("branch: unspecified")
	}
	// The upstream does not enforce restrictions on the branch name, so
	// we are fairly liberal in the alphabet and length we allow here.
	// We do impose some limits here to allow us to make some assumptions when
	// implementing UIs and RPCs.
	if err := ValidateUTF8PrintableStrict(buildID.Branch, MaxAndroidBranchLength); err != nil {
		return errors.Fmt("branch: %w", err)
	}
	if buildID.BuildId == 0 {
		return errors.New("build_id: unspecified")
	}
	if buildID.BuildId < 0 {
		return errors.New("build_id: cannot be negative")
	}
	return nil
}

// ValidateExtraBuildDescriptors validates a list of extra build descriptors.
// This performs basic structural validation only, call ValidateBuildDescriptorsUniquenessAndOrder
// to validate uniqueness is maintained and to ensure extra_builds is only set of primary_build is set.
func ValidateExtraBuildDescriptors(builds []*pb.BuildDescriptor) error {
	if len(builds) > 10 {
		return errors.New("exceeds maximum of 10 extra builds")
	}

	for i, build := range builds {
		if err := ValidateBuildDescriptor(build); err != nil {
			return errors.Fmt("[%v]: %w", i, err)
		}
	}
	return nil
}

// ValidateBuildDescriptorsUniquenessAndOrder performs extended validation on the extra builds, validating:
// - the build descriptors do not duplicate each other or the primary build
// - the extra builds field is only set of primary build is first set.
func ValidateBuildDescriptorsUniquenessAndOrder(builds []*pb.BuildDescriptor, primaryBuild *pb.BuildDescriptor) error {
	if primaryBuild == nil {
		if len(builds) > 0 {
			return errors.New("may not be specified unless primary build is set")
		}
		return nil
	}
	// Use -1 to denote the primary build.
	const primaryBuildIndex = -1

	seenKeys := make(map[string]int)
	seenKeys[keyOfBuildDescriptor(primaryBuild)] = primaryBuildIndex

	for i, build := range builds {
		key := keyOfBuildDescriptor(build)
		if idx, ok := seenKeys[key]; ok {
			if idx == primaryBuildIndex {
				return errors.Fmt("[%v]: duplicate of primary_build", i)
			} else {
				return errors.Fmt("[%v]: duplicate of extra_builds[%d]", i, idx)
			}
		}
		seenKeys[key] = i
	}
	return nil
}

func keyOfBuildDescriptor(build *pb.BuildDescriptor) string {
	switch def := build.Definition.(type) {
	case *pb.BuildDescriptor_AndroidBuild:
		return keyOfAndroidBuildDescriptor(def.AndroidBuild)
	default:
		// Should never be hit as ValidateBuildDescriptor should have
		// been called before this method is called.
		panic("definition: unspecified")
	}
}

func keyOfAndroidBuildDescriptor(build *pb.AndroidBuildDescriptor) string {
	return fmt.Sprintf("androidbuild/%q/%q/%q/%q", build.DataRealm, build.Branch, build.BuildTarget, build.BuildId)
}

// ValidateBuildDescriptor validates a BuildDescriptor.
func ValidateBuildDescriptor(build *pb.BuildDescriptor) error {
	if build == nil {
		return validate.Unspecified()
	}
	switch def := build.Definition.(type) {
	case *pb.BuildDescriptor_AndroidBuild:
		if err := ValidateAndroidBuildDescriptor(def.AndroidBuild); err != nil {
			return errors.Fmt("android_build: %w", err)
		}
	default:
		return errors.New("definition: unspecified")
	}
	return nil
}

// ValidateAndroidBuildDescriptor validates an AndroidBuildDescriptor.
func ValidateAndroidBuildDescriptor(build *pb.AndroidBuildDescriptor) error {
	if build == nil {
		return validate.Unspecified()
	}
	if build.DataRealm == "" {
		return errors.New("data_realm: unspecified")
	}
	// Support a few common data realm values from go/data-realm.
	if build.DataRealm != "prod" && build.DataRealm != "test" {
		return errors.Fmt("data_realm: unknown data realm %q", build.DataRealm)
	}

	if build.Branch == "" {
		return errors.New("branch: unspecified")
	}
	// The upstream does not enforce restrictions on the branch name, so
	// we are fairly liberal in the alphabet and length we allow here.
	// We do impose some limits here to allow us to make some assumptions when
	// implementing UIs and RPCs, such as the branch will consist only of printables
	// and is not kilobytes in size.
	if err := ValidateUTF8PrintableStrict(build.Branch, MaxAndroidBranchLength); err != nil {
		return errors.Fmt("branch: %w", err)
	}
	if build.BuildTarget == "" {
		return errors.New("build_target: unspecified")
	}
	if err := ValidateUTF8PrintableStrict(build.BuildTarget, MaxAndroidBuildTargetLength); err != nil {
		return errors.Fmt("build_target: %w", err)
	}
	if err := validate.MatchReWithLength(androidBuildIDRe, 1, MaxAndroidBuildLength, build.BuildId); err != nil {
		return errors.Fmt("build_id: %w", err)
	}
	return nil
}

func ValidateInstructions(instructions *pb.Instructions) error {
	// We allows invocation with no instructions.
	if instructions == nil {
		return nil
	}
	if proto.Size(instructions) > MaxInstructionsSize {
		return errors.Fmt("exceeds %d bytes", MaxInstructionsSize)
	}

	idMap := map[string]int{}
	for i, instruction := range instructions.Instructions {
		// Make sure that all instructions have id, and id are unique.
		if instruction.Id == "" {
			return errors.Fmt("instructions[%v]: id: unspecified", i)
		}
		if !instructionIDRe.MatchString(instruction.Id) {
			return errors.Fmt("instructions[%v]: id: does not match %q", i, instructionIDPattern)
		}
		if index, ok := idMap[instruction.Id]; ok {
			return errors.Fmt("instructions[%v]: id: %q is re-used at index %d", i, instruction.Id, index)
		}
		idMap[instruction.Id] = i
		if err := ValidateInstruction(instruction); err != nil {
			return errors.Fmt("instructions[%v]: %w", i, err)
		}
	}
	return nil
}

func ValidateInstruction(instruction *pb.Instruction) error {
	if instruction.Type == pb.InstructionType_INSTRUCTION_TYPE_UNSPECIFIED {
		return errors.New("type: unspecified")
	}
	if instruction.DescriptiveName == "" {
		return errors.New("descriptive_name: unspecified")
	}
	if len(instruction.DescriptiveName) > MaxInstructionNameSize {
		return errors.Fmt("descriptive_name: exceeds %v characters", MaxInstructionNameSize)
	}
	targetMap := map[pb.InstructionTarget]bool{}
	for i, targetedInstruction := range instruction.TargetedInstructions {
		err := ValidateTargetedInstruction(targetedInstruction, targetMap)
		if err != nil {
			return errors.Fmt("targeted_instructions[%v]: %w", i, err)
		}
	}
	// Check instruction filter.
	if err := ValidateInstructionFilter(instruction.InstructionFilter); err != nil {
		return errors.Fmt("instruction_filter: %w", err)
	}
	return nil
}

func ValidateTargetedInstruction(targetedInstruction *pb.TargetedInstruction, targetMap map[pb.InstructionTarget]bool) error {
	// Check that targets are not empty.
	if len(targetedInstruction.Targets) == 0 {
		return errors.New("targets: empty")
	}
	// Check that targets are valid.
	for i, target := range targetedInstruction.Targets {
		if target == pb.InstructionTarget_INSTRUCTION_TARGET_UNSPECIFIED {
			return errors.Fmt("targets[%v]: unspecified", i)
		}
		if _, ok := targetMap[target]; ok {
			return errors.Fmt("targets[%v]: duplicated target %q", i, target)
		}
		targetMap[target] = true
	}
	// Make sure content size <= 10KB.
	// TODO (nqmtuan): Validate this is a valid mustache template.
	if len(targetedInstruction.Content) > MaxInstructionSize {
		return errors.Fmt("content: exceeds %v characters", MaxInstructionSize)
	}
	// Check dependency.
	if err := ValidateDependencies(targetedInstruction.Dependencies); err != nil {
		return errors.Fmt("dependencies: %w", err)
	}
	return nil
}

func ValidateInstructionFilter(filter *pb.InstructionFilter) error {
	// We allow instruction without filter.
	if filter == nil {
		return nil
	}
	if filter.GetInvocationIds() != nil {
		for i, invID := range filter.GetInvocationIds().InvocationIds {
			err := ValidateInvocationID(invID)
			if err != nil {
				return errors.Fmt("invocation_ids[%v]: %w", i, err)
			}
		}
	}
	return nil
}

func ValidateDependencies(dependencies []*pb.InstructionDependency) error {
	if len(dependencies) == 0 {
		return nil
	}
	if len(dependencies) > 1 {
		return errors.New("more than 1")
	}
	for i, dep := range dependencies {
		if err := ValidateDependency(dep); err != nil {
			return errors.Fmt("[%v]: %w", i, err)
		}
	}
	return nil
}

func ValidateDependency(dependency *pb.InstructionDependency) error {
	if err := ValidateInvocationID(dependency.InvocationId); err != nil {
		return errors.Fmt("invocation_id: %w", err)
	}
	return nil
}

// SortGerritChanges sorts in-place the gerrit changes lexicographically.
func SortGerritChanges(changes []*pb.GerritChange) {
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Host != changes[j].Host {
			return changes[i].Host < changes[j].Host
		}
		if changes[i].Project != changes[j].Project {
			return changes[i].Project < changes[j].Project
		}
		if changes[i].Change != changes[j].Change {
			return changes[i].Change < changes[j].Change
		}
		return changes[i].Patchset < changes[j].Patchset
	})
}

// SourceRefFromSources extracts a SourceRef from given sources.
// This method should only be called on validated sources.
func SourceRefFromSources(srcs *pb.Sources) *pb.SourceRef {
	switch base := srcs.BaseSources.(type) {
	case *pb.Sources_GitilesCommit:
		gc := base.GitilesCommit
		return &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    gc.Host,
					Project: gc.Project,
					Ref:     gc.Ref,
				},
			},
		}
	case *pb.Sources_SubmittedAndroidBuild:
		ab := base.SubmittedAndroidBuild
		return &pb.SourceRef{
			System: &pb.SourceRef_AndroidBuild{
				AndroidBuild: &pb.AndroidBuildBranch{
					DataRealm: ab.DataRealm,
					Branch:    ab.Branch,
				},
			},
		}
	default:
		panic("unknown base sources type")
	}
}

// SourcePosition returns a source position from the given sources.
//
// The source position can only be compared with another source position
// on the same SourceRef. When comparing:
// - A larger source position means newer sources;
// - The same source position means the same sources;
// - A smaller source position means older sources.
//
// Depending on the underlying source system, the source position
// may have gaps in the numbering.
//
// This method should only be called on validated sources.
func SourcePosition(srcs *pb.Sources) int64 {
	if srcs.BaseSources == nil {
		panic("nil base sources")
	}
	switch base := srcs.BaseSources.(type) {
	case *pb.Sources_GitilesCommit:
		return base.GitilesCommit.Position
	case *pb.Sources_SubmittedAndroidBuild:
		return base.SubmittedAndroidBuild.BuildId
	default:
		panic("unknown base sources type")
	}
}

// SourceRefHash returns a short hash of the sourceRef.
//
// This method should only be called on validated sources.
func SourceRefHash(sr *pb.SourceRef) []byte {
	if sr == nil {
		panic("nil source ref")
	}
	var result [32]byte
	switch sr.System.(type) {
	case *pb.SourceRef_Gitiles:
		gitiles := sr.GetGitiles()
		result = sha256.Sum256([]byte("gitiles" + "\n" + gitiles.Host + "\n" + gitiles.Project + "\n" + gitiles.Ref))
	case *pb.SourceRef_AndroidBuild:
		android := sr.GetAndroidBuild()
		result = sha256.Sum256([]byte("androidbuild" + "\n" + android.DataRealm + "\n" + android.Branch))
	default:
		panic("invalid source ref")
	}
	return result[:8]
}

// ValidateSourceSpec validates a source specification.
func ValidateSourceSpec(sourceSpec *pb.SourceSpec) error {
	// Treat nil sourceSpec message as empty message.
	if sourceSpec.GetInherit() && sourceSpec.GetSources() != nil {
		return errors.New("only one of inherit and sources may be set")
	}
	if sourceSpec.GetSources() != nil {
		if err := ValidateSources(sourceSpec.Sources); err != nil {
			return errors.Fmt("sources: %w", err)
		}
	}
	return nil
}

// ValidateSources validates a set of sources.
func ValidateSources(sources *pb.Sources) error {
	if sources == nil {
		return errors.New("unspecified")
	}
	switch base := sources.BaseSources.(type) {
	case *pb.Sources_GitilesCommit:
		if err := ValidateGitilesCommit(base.GitilesCommit); err != nil {
			return errors.Fmt("gitiles_commit: %w", err)
		}
	case *pb.Sources_SubmittedAndroidBuild:
		if err := ValidateSubmittedAndroidBuild(base.SubmittedAndroidBuild); err != nil {
			return errors.Fmt("submitted_android_build: %w", err)
		}
	default:
		if !sources.IsDirty {
			return errors.Fmt("base_sources: unspecified; if you really have no sources (e.g. due to running tests locally) and are OK with the loss of test history for these results, set is_dirty to true")
		}
	}

	if len(sources.Changelists) > 10 {
		return errors.New("changelists: exceeds maximum of 10 changelists")
	}
	type distinctChangelist struct {
		host   string
		change int64
	}
	clToIndex := make(map[distinctChangelist]int)

	for i, cl := range sources.Changelists {
		if err := ValidateGerritChange(cl); err != nil {
			return errors.Fmt("changelists[%v]: %w", i, err)
		}
		cl := distinctChangelist{
			host:   cl.Host,
			change: cl.Change,
		}
		if duplicateIndex, ok := clToIndex[cl]; ok {
			return errors.Fmt("changelists[%v]: duplicate change modulo patchset number; same change at changelists[%v]", i, duplicateIndex)
		}
		clToIndex[cl] = i
	}
	return nil
}

// ValidateFullResourceName validates that the given resource name satisfies requirements
// of AIP-122 Full Resource Names (https://google.aip.dev/122#full-resource-names).
func ValidateFullResourceName(name string) error {
	if !strings.HasPrefix(name, "//") {
		return fmt.Errorf("resource name %q does not start with '//'", name)
	}
	if len(name) > maxResourceNameLength {
		return fmt.Errorf("resource name exceeds %d characters", maxResourceNameLength)
	}

	parsed, err := url.Parse("https:" + name)
	if err != nil {
		return fmt.Errorf("could not parse resource name %q: %w", name, err)
	}

	// The "host" part of the URL corresponds to the service name.
	// A full resource name requires a service name.
	if parsed.Host == "" {
		return fmt.Errorf("resource name %q is missing a service name", name)
	}

	// The "path" part of the URL corresponds to the resource path.
	// For a resource name, this must be non-empty.
	// This check could be enhanced to check the path is sensible (non-empty
	// segments between slashes).
	if parsed.Path == "" || parsed.Path == "/" {
		return fmt.Errorf("resource name %q is missing a resource path", name)
	}

	// AIP-122 requires that resource names be in Unicode Normalization Form C.
	if !norm.NFC.IsNormalString(name) {
		return fmt.Errorf("resource name %q is not in Unicode Normal Form C", name)
	}

	return nil
}

// ValidateProducerResource validates a producer resource reference.
func ValidateProducerResource(pr *pb.ProducerResource) error {
	if pr == nil {
		return errors.New("unspecified")
	}
	if err := ValidateProducerSystemName(pr.System); err != nil {
		return errors.Fmt("system: %w", err)
	}
	if err := validate.MatchReWithLength(producerDataRealmRE, 1, maxProducerResourceDataRealmLength, pr.DataRealm); err != nil {
		return errors.Fmt("data_realm: %w", err)
	}
	if pr.Name == "" {
		return errors.New("name: unspecified")
	}
	if len(pr.Name) > maxProducerResourceNameLength {
		return errors.Fmt("name: exceeds %d bytes", maxProducerResourceNameLength)
	}
	// AIP-122 requires that resource names be in Unicode Normalization Form C.
	if !norm.NFC.IsNormalString(pr.Name) {
		return fmt.Errorf("name: resource name %q is not in Unicode Normal Form C", pr.Name)
	}
	return nil
}

// ValidateProducerSystemName validates a producer system name.
func ValidateProducerSystemName(system string) error {
	return validate.MatchReWithLength(producerResourceSystemRE, 1, maxProducerResourceSystemLength, system)
}

// TruncateString truncates a UTF-8 string to the given number of bytes.
// If the string is truncated, ellipsis ("...") are added.
// Truncation is aware of UTF-8 runes and will only truncate whole runes.
// length must be at least 3 (to leave space for ellipsis, if needed).
func TruncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	// The index (in bytes) at which to begin truncating the string.
	lastIndex := 0
	// Find the point where we must truncate from. We only want to
	// start truncation at the start/end of a rune, not in the middle.
	// See https://blog.golang.org/strings.
	for i := range s {
		if i <= (length - 3) {
			lastIndex = i
		}
	}
	return s[:lastIndex] + "..."
}

// RemoveProducerResourceOutputOnlyFields removes output-only fields from a producer resource.
func RemoveProducerResourceOutputOnlyFields(pr *pb.ProducerResource) *pb.ProducerResource {
	if pr == nil {
		return nil
	}
	// The URL field is output only and should not be stored in Spanner.
	// Rather, it should be computed based on the current service configuration
	// whenever it is returned.
	result := proto.Clone(pr).(*pb.ProducerResource)
	result.Url = ""
	return result
}

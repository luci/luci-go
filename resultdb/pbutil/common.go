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
func SourceRefFromSources(srcs *pb.Sources) *pb.SourceRef {
	return &pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{
				Host:    srcs.GitilesCommit.Host,
				Project: srcs.GitilesCommit.Project,
				Ref:     srcs.GitilesCommit.Ref,
			},
		}}
}

// SourceRefHash returns a short hash of the sourceRef.
func SourceRefHash(sr *pb.SourceRef) []byte {
	var result [32]byte
	switch sr.System.(type) {
	case *pb.SourceRef_Gitiles:
		gitiles := sr.GetGitiles()
		result = sha256.Sum256([]byte("gitiles" + "\n" + gitiles.Host + "\n" + gitiles.Project + "\n" + gitiles.Ref))
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
	if err := ValidateGitilesCommit(sources.GetGitilesCommit()); err != nil {
		return errors.Fmt("gitiles_commit: %w", err)
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

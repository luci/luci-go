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
	"regexp"
	"sort"
	"strings"
	"time"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

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

var requestIDRe = regexp.MustCompile(`^[[:ascii:]]{0,36}$`)

// Allow hostnames permitted by
// https://www.rfc-editor.org/rfc/rfc1123#page-13. (Note that
// the 255 character limit must be seperately applied.)
var hostnameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]+(\.[a-z0-9-]+)*$`)

// The maximum hostname permitted by
// https://www.rfc-editor.org/rfc/rfc1123#page-13.
const hostnameMaxLength = 255

var sha1Regex = regexp.MustCompile(`^[a-f0-9]{40}$`)

func regexpf(patternFormat string, subpatterns ...any) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(patternFormat, subpatterns...))
}

func doesNotMatch(r *regexp.Regexp) error {
	return errors.Reason("does not match %s", r).Err()
}

func unspecified() error {
	return errors.Reason("unspecified").Err()
}

func validateWithRe(re *regexp.Regexp, value string) error {
	if value == "" {
		return unspecified()
	}
	if !re.MatchString(value) {
		return doesNotMatch(re)
	}
	return nil
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
		return doesNotMatch(requestIDRe)
	}
	return nil
}

// ValidateBatchRequestCount validates the number of requests in a batch
// request.
func ValidateBatchRequestCount(count int) error {
	const limit = 500
	if count > limit {
		return errors.Reason("the number of requests in the batch exceeds %d", limit).Err()
	}
	return nil
}

// ValidateEnum returns a non-nil error if the value is not among valid values.
func ValidateEnum(value int32, validValues map[int32]string) error {
	if _, ok := validValues[value]; !ok {
		return errors.Reason("invalid value %d", value).Err()
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
func validateProperties(properties *structpb.Struct, maxSize int) error {
	if proto.Size(properties) > maxSize {
		return errors.Reason("exceeds the maximum size of %d bytes", maxSize).Err()
	}
	return nil
}

// ValidateInvocationProperties returns a non-nil error if properties is invalid.
func ValidateInvocationProperties(properties *structpb.Struct) error {
	return validateProperties(properties, MaxSizeInvocationProperties)
}

// ValidateTestResultProperties returns a non-nil error if properties is invalid.
func ValidateTestResultProperties(properties *structpb.Struct) error {
	return validateProperties(properties, MaxSizeTestResultProperties)
}

// ValidateTestMetadataProperties returns a non-nil error if properties is invalid.
func ValidateTestMetadataProperties(properties *structpb.Struct) error {
	return validateProperties(properties, MaxSizeTestMetadataProperties)
}

// ValidateGitilesCommit validates a gitiles commit.
func ValidateGitilesCommit(commit *pb.GitilesCommit) error {
	switch {
	case commit == nil:
		return errors.Reason("unspecified").Err()

	case commit.Host == "":
		return errors.Reason("host: unspecified").Err()
	case len(commit.Host) > 255:
		return errors.Reason("host: exceeds 255 characters").Err()
	case !hostnameRE.MatchString(commit.Host):
		return errors.Reason("host: does not match %q", hostnameRE).Err()

	case commit.Project == "":
		return errors.Reason("project: unspecified").Err()
	case len(commit.Project) > hostnameMaxLength:
		return errors.Reason("project: exceeds %v characters", hostnameMaxLength).Err()

	case commit.Ref == "":
		return errors.Reason("ref: unspecified").Err()

	// The 255 character ref limit is arbitrary and not based on a known
	// restriction in Git. It exists simply because there should be a limit
	// to protect downstream clients.
	case len(commit.Ref) > 255:
		return errors.Reason("ref: exceeds 255 characters").Err()
	case !strings.HasPrefix(commit.Ref, "refs/"):
		return errors.Reason("ref: does not match refs/.*").Err()

	case commit.CommitHash == "":
		return errors.Reason("commit_hash: unspecified").Err()
	case !sha1Regex.MatchString(commit.CommitHash):
		return errors.Reason("commit_hash: does not match %q", sha1Regex).Err()

	case commit.Position == 0:
		return errors.Reason("position: unspecified").Err()
	case commit.Position < 0:
		return errors.Reason("position: cannot be negative").Err()
	}
	return nil
}

// ValidateGerritChange validates a gerrit change.
func ValidateGerritChange(change *pb.GerritChange) error {
	switch {
	case change == nil:
		return errors.Reason("unspecified").Err()

	case change.Host == "":
		return errors.Reason("host: unspecified").Err()
	case len(change.Host) > hostnameMaxLength:
		return errors.Reason("host: exceeds %v characters", hostnameMaxLength).Err()
	case !hostnameRE.MatchString(change.Host):
		return errors.Reason("host: does not match %q", hostnameRE).Err()

	case change.Project == "":
		return errors.Reason("project: unspecified").Err()
	// The 255 character project limit is arbitrary and not based on a known
	// restriction in Gerrit. It exists simply because there should be a limit
	// to protect downstream clients.
	case len(change.Project) > 255:
		return errors.Reason("project: exceeds 255 characters").Err()

	case change.Change == 0:
		return errors.Reason("change: unspecified").Err()
	case change.Change < 0:
		return errors.Reason("change: cannot be negative").Err()

	case change.Patchset == 0:
		return errors.Reason("patchset: unspecified").Err()
	case change.Patchset < 0:
		return errors.Reason("patchset: cannot be negative").Err()
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
		return errors.Reason("bigger than %d bytes", MaxInstructionsSize).Err()
	}

	idMap := map[string]int{}
	for i, instruction := range instructions.Instructions {
		// Make sure that all instructions have id, and id are unique.
		if instruction.Id == "" {
			return errors.Reason("instructions[%v]: id: unspecified", i).Err()
		}
		if index, ok := idMap[instruction.Id]; ok {
			return errors.Reason("instructions[%v]: id: %q is re-used at index %d", i, instruction.Id, index).Err()
		}
		idMap[instruction.Id] = i
		if err := ValidateInstruction(instruction); err != nil {
			return errors.Annotate(err, "instructions[%v]", i).Err()
		}
	}
	return nil
}

func ValidateInstruction(instruction *pb.Instruction) error {
	if instruction.Type == pb.InstructionType_INSTRUCTION_TYPE_UNSPECIFIED {
		return errors.Reason("type: unspecified").Err()
	}
	targetMap := map[pb.InstructionTarget]bool{}
	for i, targetedInstruction := range instruction.TargetedInstructions {
		err := ValidateTargetedInstruction(targetedInstruction, targetMap)
		if err != nil {
			return errors.Annotate(err, "targeted_instructions[%v]", i).Err()
		}
	}
	// Check instruction filter.
	if err := ValidateInstructionFilter(instruction.InstructionFilter); err != nil {
		return errors.Annotate(err, "instruction_filter").Err()
	}
	return nil
}

func ValidateTargetedInstruction(targetedInstruction *pb.TargetedInstruction, targetMap map[pb.InstructionTarget]bool) error {
	// Check that targets are not empty.
	if len(targetedInstruction.Targets) == 0 {
		return errors.Reason("targets: empty").Err()
	}
	// Check that targets are valid.
	for i, target := range targetedInstruction.Targets {
		if target == pb.InstructionTarget_INSTRUCTION_TARGET_UNSPECIFIED {
			return errors.Reason("targets[%v]: unspecified", i).Err()
		}
		if _, ok := targetMap[target]; ok {
			return errors.Reason("targets[%v]: duplicated target %q", i, target).Err()
		}
		targetMap[target] = true
	}
	// Make sure content size <= 10KB.
	// TODO (nqmtuan): Validate this is a valid mustache template.
	if len(targetedInstruction.Content) > MaxInstructionSize {
		return errors.Reason("content: longer than %v bytes", MaxInstructionSize).Err()
	}
	// Check dependency.
	if err := ValidateDependencies(targetedInstruction.Dependencies); err != nil {
		return errors.Annotate(err, "dependencies").Err()
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
				return errors.Annotate(err, "invocation_ids[%v]", i).Err()
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
		return errors.Reason("more than 1").Err()
	}
	for i, dep := range dependencies {
		if err := ValidateDependency(dep); err != nil {
			return errors.Annotate(err, "[%v]", i).Err()
		}
	}
	return nil
}

func ValidateDependency(dependency *pb.InstructionDependency) error {
	if err := ValidateInvocationID(dependency.InvocationId); err != nil {
		return errors.Annotate(err, "invocation_id").Err()
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

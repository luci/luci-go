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
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
)

const MaxSizeProperties = 4 * 1024

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

// ValidateProperties returns a non-nil error if properties is invalid.
func ValidateProperties(properties *structpb.Struct) error {
	if proto.Size(properties) > MaxSizeProperties {
		return errors.Reason("exceeds the maximum size of %d bytes", MaxSizeProperties).Err()
	}
	return nil
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

// RefHash returns a short hash of the sourceRef.
func RefHash(sr *pb.SourceRef) []byte {
	var result [32]byte
	switch sr.System.(type) {
	case *pb.SourceRef_Gitiles:
		gitiles := sr.GetGitiles()
		result = sha256.Sum256([]byte("gitiles" + "\n" + gitiles.Host + "\n" + gitiles.Project + "\n" + gitiles.Ref))
	}
	return result[:8]
}

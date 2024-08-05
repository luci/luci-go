// Copyright 2018 The LUCI Authors.
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

package protoutil

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var (
	projRegex    = regexp.MustCompile(`^[a-z0-9\-_]+$`)
	bucketRegex  = regexp.MustCompile(`^[a-z0-9\-_.]{1,100}$`)
	builderRegex = regexp.MustCompile(`^[a-zA-Z0-9\-_.\(\) ]{1,128}$`)
)

// ValidateBuilderID validates the given builder ID.
// Bucket and Builder are optional and only validated if specified.
func ValidateBuilderID(b *pb.BuilderID) error {
	switch parts := strings.Split(b.GetBucket(), "."); {
	case !projRegex.MatchString(b.GetProject()):
		return errors.Reason("project must match %q", projRegex).Err()
	case b.GetBucket() != "" && !bucketRegex.MatchString(b.Bucket):
		return errors.Reason("bucket must match %q", bucketRegex).Err()
	case b.GetBuilder() != "" && !builderRegex.MatchString(b.Builder):
		return errors.Reason("builder must match %q", builderRegex).Err()
	case b.GetBucket() != "" && parts[0] == "luci" && len(parts) > 2:
		return errors.Reason("invalid use of v1 bucket in v2 API (hint: try %q)", parts[2]).Err()
	default:
		return nil
	}
}

// ValidateRequiredBuilderID validates the given builder ID, requiring Bucket
// and Builder.
func ValidateRequiredBuilderID(b *pb.BuilderID) error {
	switch err := ValidateBuilderID(b); {
	case err != nil:
		return err
	case b.Bucket == "":
		return errors.Reason("bucket is required").Err()
	case b.Builder == "":
		return errors.Reason("builder is required").Err()
	default:
		return nil
	}
}

// ToBuilderIDString returns "{project}/{bucket}/{builder}" string.
func ToBuilderIDString(project, bucket, builder string) string {
	return fmt.Sprintf("%s/%s/%s", project, bucket, builder)
}

// FormatBuilderID converts BuilderID to a "{project}/{bucket}/{builder}" string.
func FormatBuilderID(id *pb.BuilderID) string {
	return ToBuilderIDString(id.Project, id.Bucket, id.Builder)
}

// ParseBuilderID parses a "{project}/{bucket}/{builder}" string.
// Opposite of FormatBuilderID.
func ParseBuilderID(s string) (*pb.BuilderID, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid builder id; must have 2 slashes")
	}
	return &pb.BuilderID{
		Project: parts[0],
		Bucket:  parts[1],
		Builder: parts[2],
	}, nil
}

// FormatBucketID returns "{project}/{bucket}" string.
func FormatBucketID(project, bucket string) string {
	return fmt.Sprintf("%s/%s", project, bucket)
}

// ParseBucketID parses a "{project}/{bucket}" string.
// Opposite of FormatBucketID.
func ParseBucketID(s string) (string, string, error) {
	switch parts := strings.Split(s, "/"); {
	case len(parts) != 2:
		return "", "", errors.New("invalid bucket id; must have 1 slash")
	case strings.TrimSpace(parts[0]) == "":
		return "", "", errors.New("invalid bucket id; project is empty")
	case strings.TrimSpace(parts[1]) == "":
		return "", "", errors.New("invalid bucket id; bucket is empty")
	default:
		return parts[0], parts[1], nil
	}
}

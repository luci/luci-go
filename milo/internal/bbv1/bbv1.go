// Copyright 2025 The LUCI Authors.
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

// Package bbv1 contains legacy support for Buildbucket v1 naming conventions.
package bbv1

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatBuildAddress returns a value for TagBuildAddress tag.
// If number is positive, returns "<bucket>/<builder>/<number>",
// otherwise returns "<id>"
//
// See also ParseBuildAddress.
func FormatBuildAddress(id int64, bucket, builder string, number int) string {
	if number > 0 {
		return fmt.Sprintf("%s/%s/%d", bucket, builder, number)
	}

	return strconv.FormatInt(id, 10)
}

// ParseBuildAddress parses a value of a TagBuildAddress tag.
// See also FormatBuildAddress.
//
// If id is non-zero, project, bucket and builder are zero.
// If bucket is non-zero, id is zero.
func ParseBuildAddress(address string) (id int64, project, bucket, builder string, number int, err error) {
	parts := strings.Split(address, "/")
	switch len(parts) {
	case 3:
		var numberStr string
		bucket, builder, numberStr = parts[0], parts[1], parts[2]
		project = ProjectFromBucket(bucket)
		number, err = strconv.Atoi(numberStr)
	case 1:
		id, err = strconv.ParseInt(parts[0], 10, 64)
	default:
		err = fmt.Errorf("unrecognized build address format %q", address)
	}
	return
}

// BucketNameToV2 converts a v1 Bucket name to the v2 constituent parts.
// The difference between the bucket name is that v2 uses short names, for example:
// v1: luci.chromium.try
// v2: try
// "luci" is dropped, "chromium" is recorded as the project, "try" is the name.
// If the bucket does not conform to this convention, or if it is not a luci bucket,
// then this return and empty string for both project and bucket.
func BucketNameToV2(v1Bucket string) (project string, bucket string) {
	p := strings.SplitN(v1Bucket, ".", 3)
	if len(p) != 3 || p[0] != "luci" {
		return "", ""
	}
	return p[1], p[2]
}

// ProjectFromBucket tries to retrieve project id from bucket name.
// Returns "" on failure.
func ProjectFromBucket(bucket string) string {
	// Buildbucket guarantees that buckets that start with "luci."
	// have "luci.<project id>." prefix.
	parts := strings.Split(bucket, ".")
	if len(parts) >= 3 && parts[0] == "luci" {
		return parts[1]
	}
	return ""
}

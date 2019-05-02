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

package buildbucket

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/data/strpair"
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

// ValidateBuildAddress returns an error if the build address is invalid.
func ValidateBuildAddress(address string) error {
	_, _, _, _, _, err := ParseBuildAddress(address)
	return err
}

// GetByAddress fetches a build by its address.
// Returns (nil, nil) if build is not found.
func GetByAddress(c context.Context, client *Service, address string) (*LegacyApiCommonBuildMessage, error) {
	id, _, _, _, _, err := ParseBuildAddress(address)
	if err != nil {
		return nil, err
	}

	if id != 0 {
		res, err := client.Get(id).Context(c).Do()
		switch {
		case err != nil:
			return nil, err
		case res.Error != nil && res.Error.Reason == ReasonNotFound:
			return nil, nil
		default:
			return res.Build, nil
		}
	}

	msgs, _, err := client.Search().
		Context(c).
		Tag(strpair.Format(TagBuildAddress, address)).
		IncludeExperimental(true).
		Fetch(1, nil)
	switch {
	case err != nil:
		return nil, err
	case len(msgs) == 0:
		return nil, nil
	default:
		return msgs[0], nil
	}
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

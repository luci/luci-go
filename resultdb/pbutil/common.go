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
	"fmt"
	"regexp"
	"time"

	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/errors"
)

var requestIDRe = regexp.MustCompile(`^[[:ascii:]]{0,36}$`)

func regexpf(patternFormat string, subpatterns ...interface{}) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(patternFormat, subpatterns...))
}

func doesNotMatch(r *regexp.Regexp) error {
	return errors.Reason("does not match %s", r).Err()
}

func unspecified() error {
	return errors.Reason("unspecified").Err()
}

// MustTimestampProto converts a time.Time to a *tspb.Timestamp and panics
// on failure.
func MustTimestampProto(t time.Time) *tspb.Timestamp {
	ts, err := ptypes.TimestampProto(t)
	if err != nil {
		panic(err)
	}
	return ts
}

// MustTimestamp converts a *tspb.Timestamp to a time.Time and panics
// on failure.
func MustTimestamp(ts *tspb.Timestamp) time.Time {
	t, err := ptypes.Timestamp(ts)
	if err != nil {
		panic(err)
	}
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

// ValidatePageSize returns a non-nil error if pageSize is invalid.
// Returns nil if pageSize is 0.
func ValidatePageSize(pageSize int32) error {
	if pageSize < 0 {
		return errors.Reason("negative").Err()
	}
	return nil
}

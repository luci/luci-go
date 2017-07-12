// Copyright 2016 The LUCI Authors.
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

package endpoints

import (
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/luci/luci-go/common/proto/google"
)

// MinDuration selects the smallest duration that is > 0 from a set of
// duration.Duration protobufs.
//
// If none of the supplied Durations are > 0, 0 will be returned.
func MinDuration(candidates ...*duration.Duration) (exp time.Duration) {
	for _, c := range candidates {
		if cd := google.DurationFromProto(c); cd > 0 && (exp <= 0 || cd < exp) {
			exp = cd
		}
	}
	return
}

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

package buildbucketpb

import (
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/data/strpair"
)

// RunDuration returns duration between build start and end.
func (b *Build) RunDuration() (duration time.Duration, ok bool) {
	start, startErr := ptypes.Timestamp(b.StartTime)
	end, endErr := ptypes.Timestamp(b.EndTime)
	if startErr != nil || start.IsZero() || endErr != nil || end.IsZero() {
		return 0, false
	}

	return end.Sub(start), true
}

// SchedulingDuration returns duration between build creation and start.
func (b *Build) SchedulingDuration() (duration time.Duration, ok bool) {
	create, createErr := ptypes.Timestamp(b.CreateTime)
	start, startErr := ptypes.Timestamp(b.StartTime)
	if createErr != nil || create.IsZero() || startErr != nil || start.IsZero() {
		return 0, false
	}

	return start.Sub(create), true
}

// ParseTags parses b.Tags as a strpair.Map.
func (b *Build) ParseTags() strpair.Map {
	m := make(strpair.Map, len(b.Tags))
	for _, t := range b.Tags {
		m.Add(t.Key, t.Value)
	}
	return m
}

// Buildsets returns all of the buildsets of the build.
func (b *Build) Buildsets() []string {
	result := []string{}
	for _, tag := range b.Tags {
		if tag.Key == "buildset" {
			result = append(result, tag.Value)
		}
	}
	return result
}

// Copyright 2017 The LUCI Authors.
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

package testutil

import (
	"crypto/sha1"
	"io"
	"strconv"
	"time"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/api/gitiles"
)

// TestBuild creates a minimal dummy buildbucket.Build struct for use in
// testing.
func TestBuild(creationTime time.Time, bucket, builder string, status buildbucket.Status) *buildbucket.Build {
	return &buildbucket.Build{
		Bucket:       bucket,
		Builder:      builder,
		CreationTime: creationTime,
		Status:       status,
	}
}

// TestCommit creates a minimal dummy gitiles.Commit struct for use in testing.
func TestCommit(commitTime time.Time, revision string) *gitiles.Commit {
	return &gitiles.Commit{
		Commit: revision,
		Committer: gitiles.User{
			Time: gitiles.Time(commitTime),
		},
	}
}

// TestRevision creates a random dummy revision.
func TestRevision() string {
	h := sha1.New()
	io.WriteString(h, strconv.FormatInt(time.Now().UnixNano(), 16))
	return string(h.Sum(nil)[:])
}

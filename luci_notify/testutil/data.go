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
	"fmt"
	"io"

	"go.chromium.org/luci/buildbucket/proto"
)

// TestBuild creates a minimal dummy buildbucket.Build struct for use in
// testing.
func TestBuild(project, bucket, builder string, status buildbucketpb.Status) *buildbucketpb.Build {
	return &buildbucketpb.Build{
		Builder: &buildbucketpb.Builder_ID{
			Project: project,
			Bucket:  bucket,
			Builder: builder,
		},
		Status: status,
	}
}

// TestRevision creates a dummy revision by hashing a string.
func TestRevision(seed string) string {
	h := sha1.New()
	io.WriteString(h, seed)
	return fmt.Sprintf("%x", h.Sum(nil))
}

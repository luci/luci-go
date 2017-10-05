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
	"math/rand"
	"strconv"

	"go.chromium.org/luci/buildbucket"
)

// TestBuild creates a minimal dummy buildbucket.Build struct for use in
// testing.
func TestBuild(bucket, builder string, status buildbucket.Status) *buildbucket.Build {
	return &buildbucket.Build{
		Bucket:  bucket,
		Builder: builder,
		Status:  status,
	}
}

// TestRevision creates a random dummy revision.
func TestRevision() string {
	h := sha1.New()
	io.WriteString(h, strconv.FormatInt(rand.Int63(), 16))
	return string(h.Sum(nil)[:])
}

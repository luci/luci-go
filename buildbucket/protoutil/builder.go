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
	"strings"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// FormatBuilderID returns "{project}/{bucket}/{builder}" string.
func FormatBuilderID(id *pb.BuilderID) string {
	return fmt.Sprintf("%s/%s/%s", id.Project, id.Bucket, id.Builder)
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

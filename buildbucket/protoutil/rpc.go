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

package protoutil

import (
	"fmt"
	"strconv"
	"strings"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// ParseGetBuildRequest parses a GetBuild request.
// Supports two forms:
// - <build-id>
// - <project>/<bucket>/<builder>/<build-number>
func ParseGetBuildRequest(s string) (*pb.GetBuildRequest, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		return &pb.GetBuildRequest{Id: id}, nil
	}

	parts := strings.Split(s, "/")
	if len(parts) == 4 {
		num, err := strconv.Atoi(parts[3])
		if err == nil {
			return &pb.GetBuildRequest{
				Builder: &pb.BuilderID{
					Project: parts[0],
					Bucket:  parts[1],
					Builder: parts[2],
				},
				BuildNumber: int32(num),
			}, nil
		}
	}

	return nil, fmt.Errorf(
		"build request string must be either an int64 build ID or " +
			"a <project>/<bucket>/<builder>/<build_number>, e.g. chromium/ci/linux-rel/1")
}

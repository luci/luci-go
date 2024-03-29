// Copyright 2023 The LUCI Authors.
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

// Package util contains utility functions
package util

import (
	"sort"

	pb "go.chromium.org/luci/bisection/proto/v1"
)

func SortDimension(dimensions []*pb.Dimension) {
	sort.Slice(dimensions, func(i, j int) bool {
		if dimensions[i].Key == dimensions[j].Key {
			return dimensions[i].Value < dimensions[j].Value
		}
		return dimensions[i].Key < dimensions[j].Key
	})
}

func GetDimensionWithKey(dims *pb.Dimensions, key string) *pb.Dimension {
	if dims == nil {
		return nil
	}
	for _, d := range dims.GetDimensions() {
		if d.Key == key {
			return d
		}
	}
	return nil
}

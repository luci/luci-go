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

package rpc

import (
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/stringset"
)

// valdiateUpdateMask validates the given update mask.
func validateUpdateMask(m *field_mask.FieldMask) error {
	switch {
	case m == nil:
		return status.Error(codes.InvalidArgument, "update mask is required")
	case len(m.Paths) == 0:
		return status.Error(codes.InvalidArgument, "at least one update mask path is required")
	}
	// Path names must be unique.
	// Keep records of ones we've already seen.
	paths := stringset.New(len(m.Paths))
	for _, path := range m.Paths {
		if !paths.Add(path) {
			return status.Errorf(codes.InvalidArgument, "duplicate update mask path %q", path)
		}
	}
	return nil
}

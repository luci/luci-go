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

package rpc

import (
	"path"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	pb "go.chromium.org/luci/config_service/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// validatePath validates path in rpc requests.
func validatePath(p string) error {
	if p == "" {
		return errors.New("not specified")
	}
	if path.IsAbs(p) {
		return errors.Reason("must not be absolute").Err()
	}
	if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
		return errors.Reason("should not start with './' or '../'").Err()
	}
	return nil
}

// toConfigMask converts the given field mask for Config proto to a config mask.
func toConfigMask(fields *fieldmaskpb.FieldMask) (*mask.Mask, error) {
	// Convert "content" path to "raw_content" and "signed_url", as "content" is
	// 'oneof' field type and the mask lib hasn't supported to parse it yet.
	if fieldSet := stringset.NewFromSlice(fields.GetPaths()...); fieldSet.Has("content") {
		fieldSet.Del("content")
		fieldSet.Add("raw_content")
		fieldSet.Add("signed_url")
		fields.Paths = fieldSet.ToSlice()
	}
	m, err := mask.FromFieldMask(fields, &pb.Config{}, false, false)
	return m, err
}

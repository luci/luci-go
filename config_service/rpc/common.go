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

	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	pb "go.chromium.org/luci/config_service/proto"
)

// maxRawContentSize is the max allowed raw content size per config in rpc
// responses. Any size larger than it will be responded with a GCS signed url.
const maxRawContentSize = 30 * 1024 * 1024

// defaultConfigSetMask is the default mask used for ConfigSet related RPCs.
var defaultConfigSetMask = mask.MustFromReadMask(&pb.ConfigSet{}, "name", "url", "revision")

// validatePath validates path in rpc requests.
func validatePath(p string) error {
	if p == "" {
		return errors.New("not specified")
	}
	if path.IsAbs(p) {
		return errors.New("must not be absolute")
	}
	if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") {
		return errors.New("should not start with './' or '../'")
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
	return mask.FromFieldMask(fields, &pb.Config{})
}

// toConfigSetMask converts the given field mask for ConfigSet proto to a
// ConfigSet mask.
func toConfigSetMask(fields *fieldmaskpb.FieldMask) (*mask.Mask, error) {
	if len(fields.GetPaths()) == 0 {
		return defaultConfigSetMask, nil
	}
	return mask.FromFieldMask(fields, &pb.ConfigSet{})
}

// toConfigPb converts *model.File to Config proto, excluding its content.
func toConfigPb(cs string, f *model.File) *pb.Config {
	return &pb.Config{
		ConfigSet:     cs,
		Path:          f.Path,
		ContentSha256: f.ContentSHA256,
		Size:          f.Size,
		Revision:      f.Revision.StringID(),
		Url:           common.GitilesURL(f.Location.GetGitilesLocation()),
	}
}

// toConfigSetPb converts *model.ConfigSet to ConfigSet proto.
func toConfigSetPb(cs *model.ConfigSet) *pb.ConfigSet {
	if cs == nil {
		return nil
	}
	return &pb.ConfigSet{
		Name: string(cs.ID),
		Url:  common.GitilesURL(cs.Location.GetGitilesLocation()),
		Revision: &pb.ConfigSet_Revision{
			Id:             cs.LatestRevision.ID,
			Url:            common.GitilesURL(cs.LatestRevision.Location.GetGitilesLocation()),
			CommitterEmail: cs.LatestRevision.CommitterEmail,
			AuthorEmail:    cs.LatestRevision.AuthorEmail,
			Timestamp:      timestamppb.New(cs.LatestRevision.CommitTime),
		},
	}
}

// toImportAttempt converts *model.ImportAttempt to ConfigSet_Attempt proto.
func toImportAttempt(attempt *model.ImportAttempt) *pb.ConfigSet_Attempt {
	if attempt == nil {
		return nil
	}
	return &pb.ConfigSet_Attempt{
		Message: attempt.Message,
		Success: attempt.Success,
		Revision: &pb.ConfigSet_Revision{
			Id:             attempt.Revision.ID,
			Url:            common.GitilesURL(attempt.Revision.Location.GetGitilesLocation()),
			CommitterEmail: attempt.Revision.CommitterEmail,
			AuthorEmail:    attempt.Revision.AuthorEmail,
			Timestamp:      timestamppb.New(attempt.Revision.CommitTime),
		},
		ValidationResult: attempt.ValidationResult,
	}
}

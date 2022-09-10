// Copyright 2022 The LUCI Authors.
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
	"context"
	"strings"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/protowalk"
	"go.chromium.org/luci/grpc/appstatus"
)

type CreateBuildChecker struct{}

var _ protowalk.FieldProcessor = (*CreateBuildChecker)(nil)

func (*CreateBuildChecker) Process(field protoreflect.FieldDescriptor, msg protoreflect.Message) (data protowalk.ResultData, applied bool) {
	cbfb := proto.GetExtension(field.Options().(*descriptorpb.FieldOptions), pb.E_CreateBuildFieldOption).(*pb.CreateBuildFieldOption)
	switch cbfb.FieldBehavior {
	case annotations.FieldBehavior_OUTPUT_ONLY:
		msg.Clear(field)
		return protowalk.ResultData{Message: "cleared OUTPUT_ONLY field"}, true
	case annotations.FieldBehavior_REQUIRED:
		return protowalk.ResultData{Message: "required", IsErr: true}, true
	default:
		panic("unsupported field behavior")
	}
}

func init() {
	protowalk.RegisterFieldProcessor(&CreateBuildChecker{}, func(field protoreflect.FieldDescriptor) protowalk.ProcessAttr {
		if fo := field.Options().(*descriptorpb.FieldOptions); fo != nil {
			if cbfb := proto.GetExtension(fo, pb.E_CreateBuildFieldOption).(*pb.CreateBuildFieldOption); cbfb != nil {
				switch cbfb.FieldBehavior {
				case annotations.FieldBehavior_OUTPUT_ONLY:
					return protowalk.ProcessIfSet
				case annotations.FieldBehavior_REQUIRED:
					return protowalk.ProcessIfUnset
				default:
					panic("unsupported field behavior")
				}
			}
		}
		return protowalk.ProcessNever
	})
}

func validateCreateBuildRequest(ctx context.Context, wellKnownExperiments stringset.Set, req *pb.CreateBuildRequest) (*model.BuildMask, error) {
	if procRes := protowalk.Fields(req, &protowalk.DeprecatedProcessor{}, &protowalk.OutputOnlyProcessor{}, &protowalk.RequiredProcessor{}, &CreateBuildChecker{}); procRes != nil {
		if resStrs := procRes.Strings(); len(resStrs) > 0 {
			logging.Infof(ctx, strings.Join(resStrs, ". "))
		}
		if err := procRes.Err(); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

// CreateBuild handles a request to schedule a build. Implements pb.BuildsServer.
func (*Builds) CreateBuild(ctx context.Context, req *pb.CreateBuildRequest) (*pb.Build, error) {
	_, err := validateCreateBuildRequest(ctx, nil, req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	return nil, errors.New("not implemented")
}

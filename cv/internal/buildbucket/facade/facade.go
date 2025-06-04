// Copyright 2021 The LUCI Authors.
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

package bbfacade

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/structmask"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/buildbucket"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// Facade provides APIs that LUCI CV can use to interact with buildbucket
// tryjobs.
type Facade struct {
	ClientFactory buildbucket.ClientFactory
}

func (f *Facade) Kind() string {
	return "buildbucket"
}

var defaultMask *bbpb.BuildMask

func init() {
	defaultMask = &bbpb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{},
	}
	if err := defaultMask.Fields.Append((*bbpb.Build)(nil), fieldsToParse...); err != nil {
		panic(err)
	}
	for _, key := range outputPropKeys {
		defaultMask.OutputProperties = append(defaultMask.OutputProperties, &structmask.StructMask{
			Path: []string{key},
		})
	}
}

// Fetch retrieves the Buildbucket build for the given external ID and returns
// its current status and result.
func (f *Facade) Fetch(ctx context.Context, luciProject string, eid tryjob.ExternalID) (tryjob.Status, *tryjob.Result, error) {
	host, buildID, err := eid.ParseBuildbucketID()
	if err != nil {
		return 0, nil, err
	}

	bbClient, err := f.ClientFactory.MakeClient(ctx, host, luciProject)
	if err != nil {
		return 0, nil, err
	}

	build, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{Id: buildID, Mask: defaultMask})
	switch code := status.Code(err); {
	case code == codes.OK:
		return parseStatusAndResult(ctx, build)
	case grpcutil.IsTransientCode(code) || code == codes.DeadlineExceeded:
		return 0, nil, transient.Tag.Apply(err)
	default:
		return 0, nil, err
	}
}

// Parse parses tryjob status and result from the buildbucket build.
//
// Returns error if the given data is not a buildbucket build.
func (f *Facade) Parse(ctx context.Context, data any) (tryjob.Status, *tryjob.Result, error) {
	build, ok := data.(*bbpb.Build)
	if !ok {
		return tryjob.Status_STATUS_UNSPECIFIED, nil, errors.Fmt("expected data to be *bbpb.Build, got %T", data)
	}
	return parseStatusAndResult(ctx, build)
}

// CancelTryjob asks buildbucket to cancel a running tryjob.
//
// It returns nil error if the buildbucket build is ended.
func (f *Facade) CancelTryjob(ctx context.Context, tj *tryjob.Tryjob, reason string) error {
	host, buildID, err := tj.ExternalID.ParseBuildbucketID()
	if err != nil {
		return err
	}

	bbClient, err := f.ClientFactory.MakeClient(ctx, host, tj.LUCIProject())
	if err != nil {
		return err
	}

	_, err = bbClient.CancelBuild(ctx, &bbpb.CancelBuildRequest{
		Id:              buildID,
		SummaryMarkdown: reason,
	})
	return err
}

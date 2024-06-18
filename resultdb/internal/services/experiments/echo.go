// Copyright 2024 The LUCI Authors.
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

package experiments

import (
	"context"
	"regexp"

	"go.chromium.org/luci/analysis/pbutil"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TODO(seawardt): Update to something more suitable.
const allowedGroup = "googlers"

var messageRE = regexp.MustCompile(`^[[:print:]]+$`)

func validateEchoRequest(req *pb.EchoRequest) error {
	if err := pbutil.ValidateWithRe(messageRE, req.Message); err != nil {
		return errors.Annotate(err, "message").Err()
	}
	if len(req.Message) > 1024 {
		return errors.Reason("message: exceeds 1024 bytes").Err()
	}
	return nil
}

func (*experimentsServer) Echo(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	if err := checkAllowed(ctx, allowedGroup); err != nil {
		// checkAllowed already returns an appstatus-annotated error for client errors.
		return nil, err
	}
	if err := validateEchoRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}

	// If we were doing anything with Spanner, we would manage our transaction
	// at this scope. Reads and construction of create/update/delete mutations
	// are typically handled by creating another library for each entity type.
	// See for example:
	// https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/go.chromium.org/luci/analysis/internal/clustering/rules/span.go
	//
	// ctx, cancel := span.ReadOnlyTransaction(ctx)
	// defer cancel()

	return &pb.EchoResponse{
		Message: req.Message,
	}, nil
}

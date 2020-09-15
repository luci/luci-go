// Copyright 2020 The LUCI Authors.
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

package main

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/lucictx"
)

func setResultDBContext(ctx context.Context, buildProto *bbpb.Build) (context.Context, error) {
	secrets, err := readBuildSecrets(ctx)
	if err != nil {
		return ctx, err
	}
	return lucictx.SetResultDB(ctx, &lucictx.ResultDB{
		Hostname: buildProto.Infra.Resultdb.Hostname,
		CurrentInvocation: &lucictx.ResultDBInvocation{
			Name:        buildProto.Infra.Resultdb.Invocation,
			UpdateToken: secrets.ResultdbInvocationUpdateToken,
		},
	}), nil
}

func setResultDBFromContext(ctx context.Context, buildProto *bbpb.Build) {
	if resultDBCtx := lucictx.GetResultDB(ctx); resultDBCtx != nil {
		buildProto.Infra.Resultdb = &bbpb.BuildInfra_ResultDB{
			Hostname: resultDBCtx.Hostname,
			Invocation: resultDBCtx.CurrentInvocation.Name,
		}
	}
}

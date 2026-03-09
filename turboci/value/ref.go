// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package value

import (
	"google.golang.org/protobuf/proto"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// InlineRef returns an inline ValueRef which corresponds to `vw`.
//
// This is roughly equivalent to [Inline] when given a message and a realm,
// except that this is a purely mechanical proto message assembly which cannot
// error.
func InlineRef(vw *orchestratorpb.ValueWrite) *orchestratorpb.ValueRef {
	return orchestratorpb.ValueRef_builder{
		TypeUrl: proto.String(vw.GetData().GetTypeUrl()),
		Realm:   proto.String(vw.GetRealm()),
		Inline:  vw.GetData(),
		// TODO: copy tag data when added to ValueWrite/ValueRef.
	}.Build()
}

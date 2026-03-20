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

// Inline returns a ValueRef (with inline data).
func Inline(msg proto.Message, realm string) (*orchestratorpb.ValueRef, error) {
	vw, err := Write(msg, realm)
	if err != nil {
		return nil, err
	}
	return InlineRef(vw), nil
}

// MustInline is the same as [Inline], except that it panics on error (i.e. if
// `msg` cannot be marshaled.
func MustInline(msg proto.Message, realm string) *orchestratorpb.ValueRef {
	ret, err := Inline(msg, realm)
	if err != nil {
		panic(err)
	}
	return ret
}

// AbsorbInline consumes the inline data in `ref` into `src`.
//
// Mutates `ref` to set `digest` in place of `inline`.
//
// No-op to absorb digest-based refs.
func AbsorbInline(src DataSource, ref *orchestratorpb.ValueRef) {
	if !ref.HasInline() {
		return
	}
	bin := ref.GetInline()
	dgst := ComputeDigest(bin)
	src.Intern(dgst, orchestratorpb.ValueData_builder{
		Binary: bin,
	}.Build())
	ref.SetDigest(string(dgst))
}

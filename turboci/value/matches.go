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

// WriteMatchesRef returns true if the ValueWrite and ValueRef have the
// same content.
//
// If `ref` has a digest, this will compute a digest for `write`.
func WriteMatchesRef(write *orchestratorpb.ValueWrite, ref *orchestratorpb.ValueRef) bool {
	if write.GetRealm() != ref.GetRealm() || write.GetData().GetTypeUrl() != ref.GetTypeUrl() {
		return false
	}
	if ref.HasInline() {
		return proto.Equal(write.GetData(), ref.GetInline())
	}
	return string(ComputeDigest(write.GetData())) == ref.GetDigest()
}

// RefMatchesRef returns true if the two refs match.
func RefMatchesRef(a, b *orchestratorpb.ValueRef) bool {
	if a.GetRealm() != b.GetRealm() || a.GetTypeUrl() != b.GetTypeUrl() {
		return false
	}

	const A = 0
	const B = 1
	inline := []bool{a.HasInline(), b.HasInline()}
	digest := []bool{a.HasDigest(), b.HasDigest()}

	switch {
	case inline[A] && inline[B]:
		return proto.Equal(a.GetInline(), b.GetInline())

	case inline[A] && digest[B]:
		return string(ComputeDigest(a.GetInline())) == b.GetDigest()

	case digest[A] && inline[B]:
		return a.GetDigest() == string(ComputeDigest(b.GetInline()))

	case digest[A] && digest[B]:
		return a.GetDigest() == b.GetDigest()
	}

	return false
}

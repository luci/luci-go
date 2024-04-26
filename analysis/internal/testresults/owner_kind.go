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

package testresults

import pb "go.chromium.org/luci/analysis/proto/v1"

// OwnerKindToDB encodes owner kind to its database representation.
func OwnerKindToDB(value pb.ChangelistOwnerKind) string {
	switch value {
	case pb.ChangelistOwnerKind_AUTOMATION:
		return "A"
	case pb.ChangelistOwnerKind_HUMAN:
		return "H"
	default:
		return ""
	}
}

// OwnerKindFromDB decodes owner kind from its database representation.
func OwnerKindFromDB(value string) pb.ChangelistOwnerKind {
	switch value {
	case "A":
		return pb.ChangelistOwnerKind_AUTOMATION
	case "H":
		return pb.ChangelistOwnerKind_HUMAN
	default:
		return pb.ChangelistOwnerKind_CHANGELIST_OWNER_UNSPECIFIED
	}
}

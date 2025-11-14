// Copyright 2025 The LUCI Authors.
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

// Package id has helper functions for working with TurboCI identifier messages.
//
// Of particular note is ToString/FromString and which will parse to and from
// the canonical string representation of all objects in the ids.v1 namespace.
package id

import (
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
)

// Identifier is the union of all valid Identifier messages.
type Identifier interface {
	*idspb.Identifier |
		*idspb.WorkPlan |
		*idspb.Check |
		*idspb.CheckOption |
		*idspb.CheckResult |
		*idspb.CheckResultDatum |
		*idspb.CheckEdit |
		*idspb.CheckEditOption |
		*idspb.Stage |
		*idspb.StageAttempt |
		*idspb.StageEdit
}

// ToString converts any TurboCI identifier proto into a string.
//
// The string format is defined in [identifier.proto].
//
// For stages without `is_worknode` set, this will generate string IDs like:
//
//	L123456789:?stage id
//
// These IDs are not valid for reading or writing to the graph, but are a useful
// intermediate/local representation for stages whose worknode-ness is unknown.
//
// [identifier.proto]: https://chromium.googlesource.com/infra/turboci/proto/+/refs/heads/main/turboci/graph/ids/v1/identifier.proto
func ToString[Id Identifier](id Id) string {
	// Yes, I know this is an abuse of generics, but it makes for a much better
	// API.
	//
	// In theory the compiler SHOULD also be able to optimize all of this away
	// anyway, since ToString[*blah] could have all the switches eliminated.
	//
	// I don't think it does today though :(.
	anyID := any(id)
	if wrapped, ok := anyID.(*idspb.Identifier); ok {
		switch typ := wrapped.WhichType(); typ {
		case idspb.Identifier_WorkPlan_case:
			return ToString(wrapped.GetWorkPlan())
		case idspb.Identifier_Check_case:
			return ToString(wrapped.GetCheck())
		case idspb.Identifier_CheckOption_case:
			return ToString(wrapped.GetCheckOption())
		case idspb.Identifier_CheckResult_case:
			return ToString(wrapped.GetCheckResult())
		case idspb.Identifier_CheckResultDatum_case:
			return ToString(wrapped.GetCheckResultDatum())
		case idspb.Identifier_CheckEdit_case:
			return ToString(wrapped.GetCheckEdit())
		case idspb.Identifier_CheckEditOption_case:
			return ToString(wrapped.GetCheckEditOption())
		case idspb.Identifier_Stage_case:
			return ToString(wrapped.GetStage())
		case idspb.Identifier_StageAttempt_case:
			return ToString(wrapped.GetStageAttempt())
		case idspb.Identifier_StageEdit_case:
			return ToString(wrapped.GetStageEdit())

		case idspb.Identifier_Type_not_set_case:
			return ""

		default:
			panic(fmt.Sprintf("impossible type: %s", typ))
		}
	}

	fmtVersion := func(ts *timestamppb.Timestamp) string {
		return fmt.Sprintf("%d/%d", ts.GetSeconds(), ts.GetNanos())
	}

	// will accumulate tokens in reverse order to be joined with ''.
	//
	// 4 tokens is the deepest this should ever get; double it to account for
	// separators.
	acc := make([]string, 0, 4*2)

	for stop := false; !stop; {
		switch x := anyID.(type) {
		case *idspb.WorkPlan:
			if x.HasId() {
				acc = append(acc, "L"+x.GetId())
			}
			stop = true

		case *idspb.Check:
			acc = append(acc, x.GetId(), ":C")
			anyID = x.GetWorkPlan()

		case *idspb.CheckOption:
			acc = append(acc, fmt.Sprint(x.GetIdx()), ":O")
			anyID = x.GetCheck()

		case *idspb.CheckResult:
			acc = append(acc, fmt.Sprint(x.GetIdx()), ":R")
			anyID = x.GetCheck()

		case *idspb.CheckResultDatum:
			acc = append(acc, fmt.Sprint(x.GetIdx()), ":D")
			anyID = x.GetResult()

		case *idspb.CheckEdit:
			acc = append(acc, fmtVersion(x.GetVersion()), ":V")
			anyID = x.GetCheck()

		case *idspb.CheckEditOption:
			acc = append(acc, fmt.Sprint(x.GetIdx()), ":O")
			anyID = x.GetCheckEdit()

		case *idspb.Stage:
			sep := ":?"
			if x.HasIsWorknode() {
				if x.GetIsWorknode() {
					sep = ":N"
				} else {
					sep = ":S"
				}
			}
			acc = append(acc, x.GetId(), sep)
			anyID = x.GetWorkPlan()

		case *idspb.StageAttempt:
			acc = append(acc, fmt.Sprint(x.GetIdx()), ":A")
			anyID = x.GetStage()

		case *idspb.StageEdit:
			acc = append(acc, fmtVersion(x.GetVersion()), ":V")
			anyID = x.GetStage()

		default:
			panic(fmt.Sprintf("impossible type: %T", id))
		}
	}

	slices.Reverse(acc)
	return strings.Join(acc, "")
}

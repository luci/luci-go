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

package userhtml

import (
	"fmt"

	bbutil "go.chromium.org/luci/buildbucket/protoutil"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// uiTryjob is fed to the template to draw tryjob chips.
type uiTryjob struct {
	ExternalID tryjob.ExternalID
	Definition *tryjob.Definition
	Status     tryjob.Status
	Result     *tryjob.Result
	Reused     bool
}

func makeUITryjobs(tjs []*tryjob.Tryjob, runID common.RunID) []*uiTryjob {
	// TODO(crbug/1233963):  Make sure we are not leaking any sensitive info
	// based on Read Perms. E.g. internal builder name.
	if len(tjs) == 0 {
		return nil
	}
	ret := make([]*uiTryjob, len(tjs))
	for i, tj := range tjs {
		ret[i] = &uiTryjob{
			ExternalID: tj.ExternalID,
			Definition: tj.Definition,
			Status:     tj.Status,
			Result:     tj.Result,
			Reused:     tj.LaunchedBy != runID,
		}
	}
	return ret
}

func makeUITryjobsFromSnapshots(snapshots []*tryjob.ExecutionLogEntry_TryjobSnapshot) []*uiTryjob {
	// TODO(crbug/1233963):  Make sure we are not leaking any sensitive info
	// based on Read Perms. E.g. internal builder name.
	if len(snapshots) == 0 {
		return nil
	}
	ret := make([]*uiTryjob, len(snapshots))
	for i, snapshot := range snapshots {
		ret[i] = &uiTryjob{
			ExternalID: tryjob.ExternalID(snapshot.GetExternalId()),
			Definition: snapshot.GetDefinition(),
			Status:     snapshot.GetStatus(),
			Result:     snapshot.GetResult(),
			Reused:     snapshot.GetReused(),
		}
	}
	return ret
}

// Link returns the link to the Tryjob in the external system.
func (ut *uiTryjob) Link() string {
	if ut.ExternalID == "" {
		return ""
	}
	return ut.ExternalID.MustURL()
}

// CSSClass returns a css class for styling a tryjob chip based on its
// status and its result's status.
func (ut *uiTryjob) CSSClass() string {
	switch ut.Status {
	case tryjob.Status_PENDING:
		return "not-started"
	case tryjob.Status_CANCELLED:
		return "cancelled"
	case tryjob.Status_TRIGGERED:
		return "running"
	case tryjob.Status_ENDED:
		switch ut.Result.GetStatus() {
		case tryjob.Result_RESULT_STATUS_UNSPECIFIED:
			panic("Tryjob status is ENDED but result status is not set")
		case tryjob.Result_FAILED_PERMANENTLY, tryjob.Result_FAILED_TRANSIENTLY, tryjob.Result_TIMEOUT:
			return "failed"
		case tryjob.Result_SUCCEEDED:
			return "passed"
		default:
			panic(fmt.Errorf("unexpected Tryjob result status %s", ut.Result.GetStatus()))
		}
	case tryjob.Status_UNTRIGGERED:
		return "not-started"
	default:
		panic(fmt.Errorf("unknown tryjob status: %s", ut.Status))
	}
}

// Name returns the name of the Tryjob.
//
// For Buildbucket, it is the launched builder name.
func (ut *uiTryjob) Name() string {
	// Use the result builder as authoritative because of equivalent builder. It
	// is possible that the definition of the Tryjob is (main=builder_a, equi=
	// builder_b) but the actual launched resultBuilder is builder_b.
	switch resultBuilder := ut.Result.GetBuildbucket().GetBuilder(); {
	case resultBuilder != nil:
		return bbutil.FormatBuilderID(resultBuilder)
	case ut.Definition.GetBuildbucket() != nil:
		return bbutil.FormatBuilderID(ut.Definition.GetBuildbucket().GetBuilder())
	default:
		panic(fmt.Errorf("tryjob backend %T is not supported", ut.Definition.GetBackend()))
	}
}

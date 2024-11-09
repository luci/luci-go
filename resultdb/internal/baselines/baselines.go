// Copyright 2023 The LUCI Authors.
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

// Package baselines captures baseline objects and basic interactions with them.
package baselines

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
)

// Baseline captures baseline identifiers, which map to a set of test variants.
// When determining new tests, baselines that have been created within
// 3 days are considered to be in a spin up state, and will not be able to
// calculate new tests.
type Baseline struct {
	Project         string
	BaselineID      string
	LastUpdatedTime time.Time
	CreationTime    time.Time
}

const SpinUpDuration = 72 * time.Hour

// IsSpinningUp checks whether the creation time is within 72 hours.
func (b Baseline) IsSpinningUp(now time.Time) bool {
	return now.Sub(b.CreationTime) <= SpinUpDuration
}

// NotFound is the error returned by Read if the row could not be found.
var NotFound = errors.New("baseline not found")

// Read reads the baseline with the given LUCI project and baseline ID.
// If the row does not exist, the error NotFound is returned.
func Read(ctx context.Context, project, baselineID string) (*Baseline, error) {
	key := spanner.Key{project, baselineID}

	var proj, bID string
	var lastUpdatedTime, creationTime time.Time
	err := spanutil.ReadRow(ctx, "Baselines", key, map[string]any{
		"Project":         &proj,
		"BaselineId":      &bID,
		"LastUpdatedTime": &lastUpdatedTime,
		"CreationTime":    &creationTime,
	})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			err = NotFound
		}
		return &Baseline{}, err
	}
	res := &Baseline{
		Project:         proj,
		BaselineID:      bID,
		LastUpdatedTime: lastUpdatedTime,
		CreationTime:    creationTime,
	}
	return res, nil
}

// Create returns a Spanner mutation that creates the baseline, setting CreationTime and LastUpdatedTime to spanner.CommitTimestamp
func Create(project, baselineID string) *spanner.Mutation {
	// Returns mutation that creates baseline, setting CreationTime and LastUpdatedTime to spanner.CommitTimestamp.
	row := map[string]any{
		"Project":         project,
		"BaselineId":      baselineID,
		"LastUpdatedTime": spanner.CommitTimestamp,
		"CreationTime":    spanner.CommitTimestamp,
	}
	return spanutil.InsertMap("Baselines", row)
}

// UpdateLastUpdatedTime refreshes a Baseline's LastUpdatedTime.
func UpdateLastUpdatedTime(project, baselineID string) *spanner.Mutation {
	row := map[string]any{
		"Project":         project,
		"BaselineId":      baselineID,
		"LastUpdatedTime": spanner.CommitTimestamp,
	}

	return spanutil.UpdateMap("Baselines", row)
}

// MustParseBaselineName parses a baseline name to project and baselineID.
// Panics if the name is invalid. Useful for situations when name was already
// validated.
func MustParseBaselineName(name string) (project, baselineID string) {
	project, baselineID, err := pbutil.ParseBaselineName(name)
	if err != nil {
		panic(err)
	}
	return project, baselineID
}

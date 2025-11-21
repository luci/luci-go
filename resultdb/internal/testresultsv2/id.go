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

package testresultsv2

import (
	"errors"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
)

// ID represents the primary key of TestResultsV2 table.
type ID struct {
	RootInvocationShardID rootinvocations.ShardID
	ModuleName            string
	ModuleScheme          string
	ModuleVariantHash     string
	CoarseName            string
	FineName              string
	CaseName              string
	// WorkUnitID is the identifier of the work unit
	WorkUnitID string
	ResultID   string
}

func (id ID) Validate() error {
	if id.RootInvocationShardID == (rootinvocations.ShardID{}) {
		return errors.New("RootInvocationShardID is required")
	}
	if id.ModuleName == "" {
		return errors.New("ModuleName is required")
	}
	if id.ModuleScheme == "" {
		return errors.New("ModuleScheme is required")
	}
	if id.ModuleVariantHash == "" {
		return errors.New("ModuleVariantHash is required")
	}
	if id.CaseName == "" {
		return errors.New("CaseName is required")
	}
	if id.WorkUnitID == "" {
		return errors.New("WorkUnitID is required")
	}
	if id.ResultID == "" {
		return errors.New("ResultID is required")
	}
	return nil
}

// Key returns the Spanner key.
func (id ID) Key() spanner.Key {
	return spanner.Key{
		id.RootInvocationShardID.RowID(),
		id.ModuleName,
		id.ModuleScheme,
		id.ModuleVariantHash,
		id.CoarseName,
		id.FineName,
		id.CaseName,
		id.WorkUnitID,
		id.ResultID,
	}
}

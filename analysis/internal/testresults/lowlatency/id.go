// Copyright 2026 The LUCI Authors.
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

package lowlatency

import (
	"strings"

	spanutil "go.chromium.org/luci/analysis/internal/span"
)

// RootInvocationID represents the ID of a root invocation, or a legacy invocation that is an export root.
type RootInvocationID struct {
	// True, if this ID represents a legacy invocation instead of a root invocation.
	IsLegacy bool
	// The ID of the root invocation or legacy invocation.
	Value string
}

// RowID returns the Spanner row ID corresponding to the ID.
func (id RootInvocationID) RowID() string {
	if id.IsLegacy {
		return "legacy:" + id.Value
	}
	return "root:" + id.Value
}

// RootInvocationIDFromRowID converts a Spanner row ID to RootInvocationID.
func RootInvocationIDFromRowID(rowID string) RootInvocationID {
	if strings.HasPrefix(rowID, "root:") {
		return RootInvocationID{
			Value: strings.TrimPrefix(rowID, "root:"),
		}
	}
	// Starts with "legacy:" or no prefix.
	return RootInvocationID{
		IsLegacy: true,
		Value:    strings.TrimPrefix(rowID, "legacy:"),
	}
}

// ToSpanner implements spanutil.Value.
func (id RootInvocationID) ToSpanner() any {
	return id.RowID()
}

// SpannerPtr implements spanutil.Ptr.
func (id *RootInvocationID) SpannerPtr(b *spanutil.Buffer) any {
	return &b.NullString
}

// FromSpanner implements spanutil.Ptr.
func (id *RootInvocationID) FromSpanner(b *spanutil.Buffer) error {
	*id = RootInvocationID{}
	if b.NullString.Valid {
		*id = RootInvocationIDFromRowID(b.NullString.StringVal)
	}
	return nil
}

// WorkUnitID represents the ID of a work unit (within its root invocation),
// or a legacy invocation.
type WorkUnitID struct {
	// True, if this ID represents a legacy invocation instead of a work unit.
	IsLegacy bool
	Value    string
}

// RowID returns the Spanner row ID corresponding to the ID.
func (id WorkUnitID) RowID() string {
	if id.IsLegacy {
		return "legacy:" + id.Value
	}
	return "wu:" + id.Value
}

// RootInvocationIDFromRowID converts a Spanner row ID to WorkUnitID.
func WorkUnitIDFromRowID(rowID string) WorkUnitID {
	if strings.HasPrefix(rowID, "wu:") {
		return WorkUnitID{
			Value: strings.TrimPrefix(rowID, "wu:"),
		}
	}
	// Starts with "legacy:" or no prefix.
	return WorkUnitID{
		IsLegacy: true,
		Value:    strings.TrimPrefix(rowID, "legacy:"),
	}
}

// ToSpanner implements spanutil.Value.
func (id WorkUnitID) ToSpanner() any {
	return id.RowID()
}

// SpannerPtr implements spanutil.Ptr.
func (id *WorkUnitID) SpannerPtr(b *spanutil.Buffer) any {
	return &b.NullString
}

// FromSpanner implements spanutil.Ptr.
func (id *WorkUnitID) FromSpanner(b *spanutil.Buffer) error {
	*id = WorkUnitID{}
	if b.NullString.Valid {
		*id = WorkUnitIDFromRowID(b.NullString.StringVal)
	}
	return nil
}

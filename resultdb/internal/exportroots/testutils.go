// Copyright 2024 The LUCI Authors.
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

package exportroots

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type Builder struct {
	entry ExportRoot
}

// NewBuilder starts building a new export root.
func NewBuilder(uniqifier int) *Builder {
	return &Builder{
		entry: ExportRoot{
			Invocation:            "",
			RootInvocation:        invocations.ID(fmt.Sprintf("root-%v", uniqifier)),
			IsInheritedSourcesSet: true,
			InheritedSources:      testutil.TestSourcesWithChangelistNumbers(uniqifier),
			IsNotified:            true,
			NotifiedTime:          time.Date(2023, 9, 8, 7, 6, 5, 4, time.UTC).Add(time.Duration(uniqifier) * time.Hour),
		},
	}
}

// WithInvocation sets the invocation ID.
func (b *Builder) WithInvocation(invocation invocations.ID) *Builder {
	b.entry.Invocation = invocation
	return b
}

// WithRootInvocation sets the root invocation ID.
func (b *Builder) WithRootInvocation(rootInvocation invocations.ID) *Builder {
	b.entry.RootInvocation = rootInvocation
	return b
}

// WithoutInheritedSources clears the inherited sources.
func (b *Builder) WithoutInheritedSources() *Builder {
	b.entry.IsInheritedSourcesSet = false
	b.entry.InheritedSources = nil
	return b
}

// WithInheritedSources sets the inherited sources.
func (b *Builder) WithInheritedSources(inheritedSources *pb.Sources) *Builder {
	b.entry.IsInheritedSourcesSet = true
	b.entry.InheritedSources = inheritedSources
	return b
}

// WithoutNotified clears the notified flag and time.
func (b *Builder) WithoutNotified() *Builder {
	b.entry.IsNotified = false
	b.entry.NotifiedTime = time.Time{}
	return b
}

// WithNotified sets that the export root was notified to pub/sub
// at the given time.
func (b *Builder) WithNotified(notifiedTime time.Time) *Builder {
	b.entry.IsNotified = true
	b.entry.NotifiedTime = notifiedTime
	return b
}

// Build builds the export root.
func (b *Builder) Build() ExportRoot {
	result := b.entry
	if result.InheritedSources != nil {
		// Copy inherited sources to avoid aliasing artifacts.
		result.InheritedSources = proto.Clone(result.InheritedSources).(*pb.Sources)
	}
	return b.entry
}

// SetForTesting replaces the set of stored export roots to match the given set.
func SetForTesting(ctx context.Context, exportRoots []ExportRoot) error {
	_, err := span.Apply(ctx, []*spanner.Mutation{spanner.Delete("InvocationExportRoots", spanner.AllKeys())})
	if err != nil {
		return err
	}
	// Insert the nominated export roots.
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, r := range exportRoots {
			m := spanutil.InsertMap("InvocationExportRoots", map[string]any{
				"InvocationId":          r.Invocation,
				"RootInvocationId":      r.RootInvocation,
				"IsInheritedSourcesSet": r.IsInheritedSourcesSet,
				"InheritedSources":      spanutil.Compressed(pbutil.MustMarshal(r.InheritedSources)),
				"NotifiedTime":          spanner.NullTime{Valid: r.IsNotified, Time: r.NotifiedTime},
			})
			span.BufferWrite(ctx, m)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

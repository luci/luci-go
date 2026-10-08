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

package verdict

import (
	"context"
	"fmt"
	"sync"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// FetchHierarchyAggregations queries ResultDB concurrently for test verdict counts
// at the invocation, module, coarse, and fine hierarchy levels. Errors are ignored
// so that missing or unsupported aggregations never fail the caller.
func FetchHierarchyAggregations(ctx context.Context, rdbClient pb.ResultDBClient, invID string, res *pb.TestResult, legacy bool) *format.HierarchyAggregations {
	if legacy || rdbClient == nil || invID == "" {
		return nil
	}
	normInv := base.NormalizeInvocation(invID)
	if normInv == "" {
		return nil
	}
	rootInvName := "rootInvocations/" + normInv

	aggs := &format.HierarchyAggregations{}
	var wg sync.WaitGroup

	querySingle := func(pred *pb.TestAggregationPredicate, dest **pb.TestAggregation_VerdictCounts) {
		defer wg.Done()
		resp, err := rdbClient.QueryTestAggregations(ctx, &pb.QueryTestAggregationsRequest{
			Parent:    rootInvName,
			Predicate: pred,
			PageSize:  1,
		})
		if err != nil || resp == nil || len(resp.Aggregations) == 0 {
			return
		}
		if resp.Aggregations[0].TotalVerdictCounts != nil {
			*dest = resp.Aggregations[0].TotalVerdictCounts
		}
	}

	wg.Add(1)
	go querySingle(&pb.TestAggregationPredicate{
		AggregationLevel: pb.AggregationLevel_INVOCATION,
	}, &aggs.Invocation)

	if res != nil && res.TestIdStructured != nil {
		st := res.TestIdStructured
		if st.ModuleName != "" && st.ModuleScheme != "" && st.ModuleScheme != "legacy" {
			varHash := st.ModuleVariantHash
			if varHash == "" {
				varHash = res.VariantHash
			}
			var modVariant *pb.Variant
			if varHash == "" {
				modVariant = st.ModuleVariant
				if modVariant == nil {
					modVariant = res.Variant
				}
			}

			if varHash != "" || modVariant != nil {
				wg.Add(1)
				go querySingle(&pb.TestAggregationPredicate{
					AggregationLevel: pb.AggregationLevel_MODULE,
					TestPrefixFilter: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_MODULE,
						Id: &pb.TestIdentifier{
							ModuleName:        st.ModuleName,
							ModuleScheme:      st.ModuleScheme,
							ModuleVariantHash: varHash,
							ModuleVariant:     modVariant,
						},
					},
				}, &aggs.Module)

				if st.CoarseName != "" {
					wg.Add(1)
					go querySingle(&pb.TestAggregationPredicate{
						AggregationLevel: pb.AggregationLevel_COARSE,
						TestPrefixFilter: &pb.TestIdentifierPrefix{
							Level: pb.AggregationLevel_COARSE,
							Id: &pb.TestIdentifier{
								ModuleName:        st.ModuleName,
								ModuleScheme:      st.ModuleScheme,
								ModuleVariantHash: varHash,
								ModuleVariant:     modVariant,
								CoarseName:        st.CoarseName,
							},
						},
					}, &aggs.Coarse)
				}

				if st.FineName != "" {
					wg.Add(1)
					go querySingle(&pb.TestAggregationPredicate{
						AggregationLevel: pb.AggregationLevel_FINE,
						TestPrefixFilter: &pb.TestIdentifierPrefix{
							Level: pb.AggregationLevel_FINE,
							Id: &pb.TestIdentifier{
								ModuleName:        st.ModuleName,
								ModuleScheme:      st.ModuleScheme,
								ModuleVariantHash: varHash,
								ModuleVariant:     modVariant,
								CoarseName:        st.CoarseName,
								FineName:          st.FineName,
							},
						},
					}, &aggs.Fine)
				}
			} else {
				wg.Add(1)
				go querySingle(&pb.TestAggregationPredicate{
					AggregationLevel: pb.AggregationLevel_MODULE,
					ContentsFilter:   fmt.Sprintf("test_id_structured.module_name = %q", st.ModuleName),
				}, &aggs.Module)
			}
		}
	}

	wg.Wait()
	return aggs
}

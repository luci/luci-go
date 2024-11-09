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

package testfailuredetection

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/bisection/model"
)

// First sorts the input slice and returns the first element in the sorted slice.
func First(ctx context.Context, bundles []*model.TestFailureBundle) *model.TestFailureBundle {
	Sort(bundles)
	// Log the sorted bundles for debugging purpose.
	lines := []string{}
	for i, b := range bundles {
		tf := b.Primary()
		line := fmt.Sprintf("%d. primary test %s(%s), redundancy score %f, regression end position %d", i, tf.TestID, tf.VariantHash, tf.RedundancyScore, tf.RegressionEndPosition)
		lines = append(lines, line)
	}
	logging.Infof(ctx, fmt.Sprintf("sorted bundles\n%s", strings.Join(lines, "\n")))
	return bundles[0]
}

// Sort sorts slice of testFailureBundles by their
// redundancy score and regression end position.
func Sort(bundles []*model.TestFailureBundle) {
	sort.Sort(sortableTestFailureBundles(bundles))
}

type sortableTestFailureBundles []*model.TestFailureBundle

// Len is the number of elements in the collection.
func (s sortableTestFailureBundles) Len() int {
	return len(s)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (s sortableTestFailureBundles) Less(i int, j int) bool {
	// We sort test bundle by recency descendingly first (truncated to day), and by redundancy score ascendingly.
	di := s[i].Primary().StartHour.Truncate(time.Hour * 24)
	dj := s[j].Primary().StartHour.Truncate(time.Hour * 24)
	return di.After(dj) || (di == dj && s[i].Primary().RedundancyScore < s[j].Primary().RedundancyScore)
}

// Swap swaps the elements with indexes i and j.
func (s sortableTestFailureBundles) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}

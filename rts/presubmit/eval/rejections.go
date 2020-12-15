// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"context"
	"sort"

	"go.chromium.org/luci/common/errors"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

type analyzedRejection struct {
	*evalpb.Rejection
	// Affectedness are affectedness for each element of Rejection.FailedTestVariants,
	// in the same order.
	Affectedness AffectednessSlice
	// Closest is the closest element in Affectedness.
	// See also AffectednessSlice.closest().
	Closest Affectedness
}

func analyzeRejections(ctx context.Context, rejC <-chan *evalpb.Rejection, strategy Strategy, callback func(context.Context, analyzedRejection) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case rej, ok := <-rejC:
			if !ok {
				return nil
			}

			// TODO(crbug.com/1112125): skip the patchset if it has a ton of failed tests.
			// Most selection strategies would reject such a patchset, so it represents noise.

			// Select tests.
			in := Input{
				TestVariants: rej.FailedTestVariants,
			}
			in.ensureChangedFilesInclude(rej.Patchsets...)
			out := &Output{
				TestVariantAffectedness: make(AffectednessSlice, len(in.TestVariants)),
			}
			if err := strategy(ctx, in, out); err != nil {
				return errors.Annotate(err, "the selection strategy failed").Err()
			}

			closest, err := out.TestVariantAffectedness.closest()
			if err != nil {
				return err
			}

			err = callback(ctx, analyzedRejection{
				Rejection:    rej,
				Affectedness: out.TestVariantAffectedness,
				Closest:      out.TestVariantAffectedness[closest],
			})
			if err != nil {
				return err
			}
		}
	}
}

type rejectionPrinter struct {
	*printer
}

// printRejection prints a rejection.
func (p *rejectionPrinter) rejection(rej *evalpb.Rejection) error {
	pf := p.printf

	pf("Lost rejection:\n")
	p.Level++

	// Print patchsets.
	if len(rej.Patchsets) == 1 {
		p.patchset(rej.Patchsets[0])
	} else {
		pf("- patchsets:\n")
		p.Level++
		for _, ps := range rej.Patchsets {
			p.patchset(ps)
		}
		p.Level--
	}

	p.testVariant(rej.FailedTestVariants)

	p.Level--
	return p.err
}

func (p *rejectionPrinter) patchset(ps *evalpb.GerritPatchset) {
	p.printf("%s\n", psURL(ps))

	paths := make([]string, len(ps.ChangedFiles))
	for i, f := range ps.ChangedFiles {
		paths[i] = f.Path
	}
	sort.Strings(paths)

	p.Level++
	for _, f := range paths {
		p.printf("%s\n", f)
	}
	p.Level--
}

// printTestVariants prints tests grouped by variant.
func (p *rejectionPrinter) testVariant(testVariants []*evalpb.TestVariant) {
	pf := p.printf

	// Group by variant.
	byVariant := map[string][]*evalpb.TestVariant{}
	var keys []string
	for _, tv := range testVariants {
		key := variantString(tv.Variant)
		tests, ok := byVariant[key]
		byVariant[key] = append(tests, tv)
		if !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	// Print tests grouped by variant.
	pf("Failed and not selected tests:\n")
	p.Level++
	for _, key := range keys {
		pf("- ")
		if key == "" {
			pf("<empty test variant>\n")
		} else {
			pf("%s\n", key)
		}

		ts := byVariant[key]
		sort.Slice(ts, func(i, j int) bool {
			return ts[i].Id < ts[j].Id
		})

		p.Level++
		for _, t := range ts {
			p.printf("- %s\n", t.Id)
			if t.FileName != "" {
				p.printf("  in %s\n", t.FileName)
			}
		}
		p.Level--
	}
	p.Level--
}

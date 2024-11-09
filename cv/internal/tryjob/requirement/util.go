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

package requirement

import (
	"crypto"
	"encoding/binary"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/cv/internal/run"
)

var (
	// Reference: https://chromium.googlesource.com/infra/luci/luci-py/+/a6655aa3/appengine/components/components/config/proto/service_config.proto#87
	projectRE = `[a-z0-9\-]+`
	// Reference: https://chromium.googlesource.com/infra/luci/luci-go/+/6b8fdd66/buildbucket/proto/project_config.proto#482
	bucketRE = `[a-z0-9\-_.]+`
	// Reference: https://chromium.googlesource.com/infra/luci/luci-go/+/6b8fdd66/buildbucket/proto/project_config.proto#220
	builderRE                 = `[a-zA-Z0-9\-_.\(\) ]+`
	modernProjBucketRe        = fmt.Sprintf(`%s/%s`, projectRE, bucketRE)
	legacyProjBucketRe        = fmt.Sprintf(`luci\.%s\.%s`, projectRE, bucketRE)
	buildersRE                = fmt.Sprintf(`((%s)|(%s))\s*:\s*%s(\s*,\s*%s)*`, modernProjBucketRe, legacyProjBucketRe, builderRE, builderRE)
	tryjobDirectiveLineRegexp = regexp.MustCompile(fmt.Sprintf(`^\s*%s(\s*;\s*%s)*\s*$`, buildersRE, buildersRE))
)

// parseTryjobDirectives parses a list of builders from the Tryjob directives
// lines provided via git footers like `Cq-Include-Trybots` and
// `Override-Tryjobs-For-Automation`.
//
// Return a list of builders.
func parseTryjobDirectives(directives []string) (stringset.Set, ComputationFailure) {
	ret := make(stringset.Set)
	for _, d := range directives {
		builderStrings, compFail := parseBuilderStrings(d)
		if compFail != nil {
			return nil, compFail
		}
		for _, builderString := range builderStrings {
			ret.Add(builderString)
		}
	}
	return ret, nil
}

// TODO(robertocn): Consider moving the parsing of the Tryjob directives
// like `Cq-Include-Trybots` to the place where the footer values are
// extracted, and refactor the corresponding field in RunOptions accordingly
// (e.g. to be a list of builder ids).
func parseBuilderStrings(line string) ([]string, ComputationFailure) {
	if !tryjobDirectiveLineRegexp.MatchString(line) {
		return nil, &invalidTryjobDirectives{line}
	}
	var ret []string
	for _, bucketSegment := range strings.Split(strings.TrimSpace(line), ";") {
		parts := strings.Split(strings.TrimSpace(bucketSegment), ":")
		if len(parts) != 2 {
			panic(fmt.Errorf("impossible; expected %q separated by exactly one \":\", got %d", bucketSegment, len(parts)-1))
		}
		projectBucket, builders := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		var project, bucket string
		if strings.HasPrefix(projectBucket, "luci.") {
			// Legacy style. Example: luci.chromium.try: builder_a
			parts := strings.SplitN(projectBucket, ".", 3)
			project, bucket = parts[1], parts[2]
		} else {
			// Modern style. Example: chromium/try: builder_a
			parts := strings.SplitN(projectBucket, "/", 2)
			project, bucket = parts[0], parts[1]
		}
		for _, builderName := range strings.Split(builders, ",") {
			ret = append(ret, fmt.Sprintf("%s/%s/%s", strings.TrimSpace(project), strings.TrimSpace(bucket), strings.TrimSpace(builderName)))
		}
	}
	return ret, nil
}

// makeRands makes `n` new pseudo-random generators to be used for the Tryjob
// Requirement computation's random selections (e.g. equivalentBuilder)
//
// The generators are seeded deterministically based on the set of CLs (their
// IDs, specifically) and their trigger times.
//
// We do it this way so that recomputing the Requirement for the same Run yields
// the same result, but triggering the same set of CLs subsequent times has a
// chance of generating a different set of random selections.
func makeRands(in Input, n int) []*rand.Rand {
	// Though MD5 is cryptographically broken, it's not being used here for
	// security purposes, and it's faster than SHA.
	h := crypto.MD5.New()
	buf := make([]byte, 8)
	cls := make([]*run.RunCL, len(in.CLs))
	copy(cls, in.CLs)
	sort.Slice(cls, func(i, j int) bool { return cls[i].ID < cls[j].ID })
	var err error
	for _, cl := range cls {
		binary.LittleEndian.PutUint64(buf, uint64(cl.ID))
		_, err = h.Write(buf)
		if err == nil && cl.Trigger != nil {
			binary.LittleEndian.PutUint64(buf, uint64(cl.Trigger.Time.AsTime().UTC().Unix()))
			_, err = h.Write(buf)
		}
		if err != nil {
			panic(err)
		}
	}
	digest := h.Sum(nil)
	// Use the first eight bytes of the digest to seed a new rand, and use such
	// rand's generated values to seed the requested number of generators.
	baseRand := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(digest[:8]))))
	ret := make([]*rand.Rand, n)
	for i := range ret {
		ret[i] = rand.New(rand.NewSource(baseRand.Int63()))
	}
	return ret
}

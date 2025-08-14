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

package main

import (
	"bufio"
	"flag"
	"reflect"
	"strings"
	"testing"
)

func TestParseTimings(t *testing.T) {
	t.Parallel()

	output := `
Top 1 slowest examples (0.224 seconds, 100.0% of total time):
  Test OverviewTab component given a project and cluster ID, should show problems and cluster history for that cluster
    0.224 seconds ./src/clusters/components/cluster/cluster_analysis_section/overview_tab/overview_tab.test.tsx
Test Suites: 1 passed, 1 total
Time: 5.588 s
`
	scanner := bufio.NewScanner(strings.NewReader(output))
	testTimings := make(map[string][]float64)
	var allTimings []float64

	parseTimings(scanner, testTimings, &allTimings)

	if len(testTimings) != 1 {
		t.Errorf("expected 1 test timing, got %d", len(testTimings))
	}
	expectedTimings := []float64{0.224}
	if !reflect.DeepEqual(testTimings["Test OverviewTab component given a project and cluster ID, should show problems and cluster history for that cluster"], expectedTimings) {
		t.Errorf("expected timings to be %v, got %v", expectedTimings, testTimings["Test OverviewTab component given a project and cluster ID, should show problems and cluster history for that cluster"])
	}

	if len(allTimings) != 1 {
		t.Errorf("expected 1 total timing, got %d", len(allTimings))
	}
	if allTimings[0] != 5588.0 {
		t.Errorf("expected total timing to be 5588.0, got %f", allTimings[0])
	}
}

func TestArgsParsing(t *testing.T) {
	// This test can't run in parallel because it modifies os.Args.

	// Test case 1: No args
	_, err := parseArgs([]string{})
	if err == nil {
		t.Errorf("expected error when no args are provided")
	}

	// Test case 2: Only matcher
	matcher, err := parseArgs([]string{"some-matcher"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if matcher != "some-matcher" {
		t.Errorf("expected matcher to be 'some-matcher', got '%s'", matcher)
	}
	if *runs != 50 {
		t.Errorf("expected runs to be 50, got %d", *runs)
	}
	if *verbose != false {
		t.Errorf("expected verbose to be false, got %t", *verbose)
	}
	if *perf != false {
		t.Errorf("expected perf to be false, got %t", *perf)
	}

	// Test case 3: All flags
	flag.CommandLine = flag.NewFlagSet("test", flag.ExitOnError)
	runs = flag.Int("n", 50, "")
	verbose = flag.Bool("v", false, "")
	perf = flag.Bool("p", false, "")
	matcher, err = parseArgs([]string{"-n", "10", "-v", "-p", "some-matcher"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if matcher != "some-matcher" {
		t.Errorf("expected matcher to be 'some-matcher', got '%s'", matcher)
	}
	if *runs != 10 {
		t.Errorf("expected runs to be 10, got %d", *runs)
	}
	if *verbose != true {
		t.Errorf("expected verbose to be true, got %t", *verbose)
	}
	if *perf != true {
		t.Errorf("expected perf to be true, got %t", *perf)
	}
}

func TestPercentile(t *testing.T) {
	t.Parallel()

	data := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	p50 := percentile(data, 50)
	if p50 != 5.5 {
		t.Errorf("expected p50 to be 5.5, got %f", p50)
	}

	p90 := percentile(data, 90)
	if p90 != 9.1 {
		t.Errorf("expected p90 to be 9.1, got %f", p90)
	}

	p99 := percentile(data, 99)
	if p99 != 9.91 {
		t.Errorf("expected p99 to be 9.91, got %f", p99)
	}

	emptyData := []float64{}
	p50Empty := percentile(emptyData, 50)
	if p50Empty != 0.0 {
		t.Errorf("expected p50 for empty data to be 0.0, got %f", p50Empty)
	}
}

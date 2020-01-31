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

package backend

import (
	"time"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// A copy of typepb.StringPair, suitable for representing a key:value pair in
// a BQ table.
// Inferred to be a field of type RECORD with Key and Value string fields.
type StringPair struct {
	Key   string
	Value string
}

// A subset of pb.Invocation for the invocation fields that need to be saved in
// a BQ table.
type Invocation struct {
	// Id of the invocation.
	Id string

	// Whether the invocation is interrupted or not.
	// For more details, refer to pb.Invocation.Interrupted.
	Interrupted bool

	// Invocation-level string key-value pairs.
	// A key can be repeated.
	Tags []*StringPair `bigquery:",nullable"`
}

// A struct for test exoneration related information.
type TestExoneration struct {
	Exonerated bool
}

// A subset of pb.Artifact for the artifact fields that need to be saved in
// a BQ table.
type Artifact struct {
	// A slash-separated relative path, identifies the artifact.
	Name string

	// Media type of the artifact.
	ContentType string
}

// A subset of pb.TestResult representing a row in a BigQuery table for result
// of a functional test case.
type TestResultRow struct {
	// Info of the exported invocation.
	// Note that it's possible that this invocation is not the result's
	// immediate parent invocation, but the including invocation.
	Invocation Invocation

	// Whether the test variant is exonerated.
	// Note that the exoneration is at the test variant level, not result level.
	TestExoneration TestExoneration

	// A unique identifier of the test in a LUCI project.
	// Refer to pb.TestResult.TestId for details.
	TestId string

	// Description of one specific way of running the test,
	//  e.g. a specific bucket, builder and a test suite.
	Variant []*StringPair

	// Whether the result of test case execution is expected.
	// Refer to pb.TestResult.Expected for details.
	Expected bool

	// Machine-readable status of the test case.
	Status pb.TestStatus

	// Human-readable explanation of the result, in HTML.
	SummaryHtml string

	// The point in time when the test case started to execute.
	StartTime time.Time

	// Duration of the test case execution.
	Duration time.Duration

	// Metadata for this test result.
	// It might describe this particular execution or the test case.
	Tags []*StringPair `bigquery:",nullable"`

	// Artifacts consumed by this test result.
	InputArtifacts []*Artifact

	// Artifacts produced by this test result.
	OutputArtifacts []*Artifact
}

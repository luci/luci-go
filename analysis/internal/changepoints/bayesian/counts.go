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

package bayesian

import "go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"

// Stores counts of test run outcomes.
type counts struct {
	// The total number of test runs. A run corresponds to a
	// single ResultDB invocation and the test results directly
	// included in it (e.g. swarming task).
	// Each run can have have one or more test results, each of
	// which we will call a 'try'.
	Runs int

	// The number of test runs which contained at least one
	// unexpected test result.
	//
	// The ratio HasUnexpected / Runs is the ratio that
	// explains the test's tendency to produce runs with
	// unexpected result(s).
	HasUnexpected int

	// The number of times a test run contained at least one
	// unexpected result AND had additional test results (i.e. had
	// an unexpected result AND two or more test results in total).
	Retried int

	// The number of times a test run contained only
	// unexpected test results AND had at least two unexpected
	// results (i.e. was unexpected after all retries).
	//
	// The ratio UnexpectedAfterRetry / Retried measures the
	// consistency / stickiness of the test's unexpected results,
	// that is, the tendency for it to continue producing unexpected
	// results if there is one unexpected result. The inverse of
	// consistency is often called 'flakiness'.
	UnexpectedAfterRetry int
}

func (h counts) addRun(run inputbuffer.Run) counts {
	h.Runs += 1
	if run.Unexpected.Count() == 0 {
		return h
	}
	h.HasUnexpected += 1
	if run.Expected.Count()+run.Unexpected.Count() < 2 {
		return h
	}
	h.Retried += 1
	if run.Expected.Count() > 0 {
		return h
	}
	h.UnexpectedAfterRetry += 1
	return h
}

func (h counts) add(other counts) counts {
	h.Runs += other.Runs
	h.HasUnexpected += other.HasUnexpected
	h.Retried += other.Retried
	h.UnexpectedAfterRetry += other.UnexpectedAfterRetry
	return h
}

func (h counts) subtract(other counts) counts {
	h.Runs -= other.Runs
	h.HasUnexpected -= other.HasUnexpected
	h.Retried -= other.Retried
	h.UnexpectedAfterRetry -= other.UnexpectedAfterRetry

	if h.Runs < 0 || h.HasUnexpected < 0 || h.Retried < 0 || h.UnexpectedAfterRetry < 0 {
		// This indicates a logic error somewhere.
		panic("subtraction resulted in value smaller than zero")
	}
	return h
}

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

package exporter

import (
	"testing"

	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPersistentHashTestIdentifier(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    *bqpb.AntsTestResultRow_TestIdentifier
		expected string
	}{
		{
			name:     "empty fields, no params",
			input:    &bqpb.AntsTestResultRow_TestIdentifier{},
			expected: "de47c9b27eb8d300dbb5f2c353e632c393262cf06340c4fa7f1b40c4cbd36f90",
		},
		{
			name: "populated fields with single param",
			input: &bqpb.AntsTestResultRow_TestIdentifier{
				Module:    "moduleA",
				TestClass: "testClassA",
				Method:    "methodA",
			},
			expected: "34824e54b5a9bac18879af82909924da50635a332fefc5c3638a0e957ae49c80",
		},
		{
			name: "populated all",
			input: &bqpb.AntsTestResultRow_TestIdentifier{
				Module: "moduleA",
				ModuleParameters: []*bqpb.StringPair{
					{Name: "module-abi", Value: "x86_64"},
				},
				ModuleParametersHash: "fb333bd62d7e6ce27603606218c06b136d20d6d3a958223281592af645af9c1e",
				TestClass:            "com.android.testClassA",
				Method:               "methodA",
			},
			expected: "591c0741b41321c9ce84a6f999e63581f5fd883f2cb20a3068190ca787478269",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual, err := persistentHashTestIdentifier(tc.input)
			assert.NoErr(t, err)
			assert.Loosely(t, actual, should.Equal(tc.expected))
		})
	}
}

func TestHashParameters(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    []*bqpb.StringPair
		expected string
	}{
		{
			name:     "nil slice",
			input:    nil,
			expected: "",
		},
		{
			name: "single parameter",
			input: []*bqpb.StringPair{
				{Name: "n2.2", Value: "v2.2"},
			},
			expected: "f299baffc34885241144be98e24a6d93acb3a0f0675ca3149e28393183c8e2f4",
		},
		{
			name: "out of order parameter",
			input: []*bqpb.StringPair{
				{Name: "n1.2", Value: "v1.2"},
				{Name: "n1.1", Value: "v1.1"},
			},
			expected: "c8b23f4f74e3b1556aca1ad21e6287731a93cea8edab9b4b80bce6e841e7c863",
		},
		{
			name: "in order parameter",
			input: []*bqpb.StringPair{
				{Name: "n1.1", Value: "v1.1"},
				{Name: "n1.2", Value: "v1.2"},
			},
			expected: "c8b23f4f74e3b1556aca1ad21e6287731a93cea8edab9b4b80bce6e841e7c863",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			actual := hashParameters(tc.input)
			assert.Loosely(t, actual, should.Equal(tc.expected))
		})
	}
}

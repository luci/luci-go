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

package testresultsv2

import (
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/aip160"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/span"
)

func TestTestIDColumn(t *testing.T) {
	ftt.Run("TestIDColumn", t, func(t *ftt.Test) {
		fieldNames := StructuredTestIDColumnNames{
			ModuleName:   "ModuleName",
			ModuleScheme: "ModuleScheme",
			CoarseName:   "T1CoarseName",
			FineName:     "T2FineName",
			CaseName:     "T3CaseName",
		}
		table := aip160.NewDatabaseTable().WithFields(
			aip160.NewField().WithFieldPath("test_id").WithBackend(
				NewFlatTestIDFieldBackend(fieldNames),
			).Filterable().Build(),
		).Build()

		t.Run("equals operator", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`test_id = ":module!scheme:coarse_name:fine_name#case_name"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("(T.ModuleName = @p_0 AND T.ModuleScheme = @p_1 AND T.T1CoarseName = @p_2 AND T.T2FineName = @p_3 AND T.T3CaseName = @p_4)"))
			assert.Loosely(t, pars, should.Match([]aip160.SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "module",
				},
				{
					Name:  "p_1",
					Value: "scheme",
				},
				{
					Name:  "p_2",
					Value: "coarse_name",
				},
				{
					Name:  "p_3",
					Value: "fine_name",
				},
				{
					Name:  "p_4",
					Value: "case_name",
				},
			}))
		})

		t.Run("not equals operator", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`test_id != "legacy_id"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("(NOT (T.ModuleName = @p_0 AND T.ModuleScheme = @p_1 AND T.T1CoarseName = @p_2 AND T.T2FineName = @p_3 AND T.T3CaseName = @p_4))"))
			assert.Loosely(t, pars, should.Match([]aip160.SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "legacy",
				},
				{
					Name:  "p_1",
					Value: "legacy",
				},
				{
					Name:  "p_2",
					Value: "",
				},
				{
					Name:  "p_3",
					Value: "",
				},
				{
					Name:  "p_4",
					Value: "legacy_id",
				},
			}))
		})

		t.Run("argument other than constant string used", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`test_id = 5`)
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "test_id": expected a quoted ("") string literal but got possible field reference "5", did you mean to wrap the value in quotes?`))
		})
		t.Run("invalid test ID used", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`test_id = ":!invalid"`)
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "test_id": unexpected end of string at byte 9, expected delimiter ':' (test ID pattern is :module!scheme:coarse:fine#case)`))
		})

		t.Run("partial match operator", func(t *ftt.Test) {
			t.Run("simple (grammarless) substring", func(t *ftt.Test) {
				// Expect to match against each field in isolation and as a legacy test ID.
				expectedQuery := strings.Join([]string{
					`(((T.ModuleName LIKE @p_0 OR T.ModuleScheme LIKE @p_1 OR T.T1CoarseName LIKE @p_2 OR T.T2FineName LIKE @p_3 OR T.T3CaseName LIKE @p_4) AND T.ModuleName <> "legacy") OR `,
					`(T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_5))`,
				}, "")

				t.Run("basic", func(t *ftt.Test) {
					filter, err := aip160.ParseFilter(`test_id:"substring"`)
					assert.Loosely(t, err, should.BeNil)

					result, pars, err := table.WhereClause(filter, "T", "p_")
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, result, should.Equal(expectedQuery))

					// Check parameters.
					assert.Loosely(t, pars, should.HaveLength(6))
					assert.Loosely(t, pars[0].Value, should.Equal("%substring%")) // ModuleName
					assert.Loosely(t, pars[1].Value, should.Equal("%substring%")) // ModuleScheme
					assert.Loosely(t, pars[2].Value, should.Equal("%substring%")) // CoarseName
					assert.Loosely(t, pars[3].Value, should.Equal("%substring%")) // FineName
					assert.Loosely(t, pars[4].Value, should.Equal("%substring%")) // CaseName
					assert.Loosely(t, pars[5].Value, should.Equal("%substring%")) // Legacy param
				})
				t.Run("escaping", func(t *ftt.Test) {
					filter, err := aip160.ParseFilter(`test_id:"sub_%string\\:\\#\\!\\\\"`)
					assert.Loosely(t, err, should.BeNil)

					result, pars, err := table.WhereClause(filter, "T", "p_")
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, result, should.Equal(expectedQuery))

					// Check parameters.
					// Note in LIKE expressions, `\`, `_` and % needs to be escaped as `\\` to match
					// a literal `\`, `_` or `%` character.
					// In the case name, "\" and ":" (not intended as a extended test ID hierarchy
					// separators or to escape are escaped with a "\").
					assert.Loosely(t, pars, should.HaveLength(6))
					assert.Loosely(t, pars[0].Value, should.Equal(`%sub\_\%string:#!\\%`))         // ModuleName
					assert.Loosely(t, pars[1].Value, should.Equal(`%sub\_\%string:#!\\%`))         // ModuleScheme
					assert.Loosely(t, pars[2].Value, should.Equal(`%sub\_\%string:#!\\%`))         // CoarseName
					assert.Loosely(t, pars[3].Value, should.Equal(`%sub\_\%string:#!\\%`))         // FineName
					assert.Loosely(t, pars[4].Value, should.Equal(`%sub\_\%string\\:#!\\\\%`))     // CaseName
					assert.Loosely(t, pars[5].Value, should.Equal(`%sub\_\%string\\:\\#\\!\\\\%`)) // Legacy param
				})
			})
			t.Run("complete test ID", func(t *ftt.Test) {
				filter, err := aip160.ParseFilter(`test_id:":module!gtest::fine#case_name"`)
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "T", "p_")
				assert.Loosely(t, err, should.BeNil)

				// This could be part of:
				// - a structured test ID with possible extension, e.g. `:module!gtest::fine#case_name_continued`
				// - a structured test ID with extension before and possibly after, e.g. `:path/to/directory\:module!gtest::fine#case_name_continued`
				// - a legacy test ID, e.g. `legacytest:module!gtest::fine#case_name_continued`
				assert.Loosely(t, result, should.Equal(strings.Join([]string{
					`(((T.ModuleName = @p_0 AND T.ModuleScheme = @p_1 AND T.T1CoarseName = @p_2 AND T.T2FineName = @p_3 AND STARTS_WITH(T.T3CaseName, @p_4)) AND T.ModuleName <> "legacy") OR `,
					`((ENDS_WITH(T.ModuleName, @p_5) AND T.ModuleScheme = @p_6 AND T.T1CoarseName = @p_7 AND T.T2FineName = @p_8 AND STARTS_WITH(T.T3CaseName, @p_9)) AND T.ModuleName <> "legacy") OR `,
					`(T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_10))`,
				}, "")))
				assert.Loosely(t, pars, should.HaveLength(11))
				assert.Loosely(t, pars[0].Value, should.Equal("module"))
				assert.Loosely(t, pars[1].Value, should.Equal("gtest"))
				assert.Loosely(t, pars[2].Value, should.Equal(""))
				assert.Loosely(t, pars[3].Value, should.Equal("fine"))
				assert.Loosely(t, pars[4].Value, should.Equal("case_name"))
				assert.Loosely(t, pars[5].Value, should.Equal(":module"))
				assert.Loosely(t, pars[6].Value, should.Equal("gtest"))
				assert.Loosely(t, pars[7].Value, should.Equal(""))
				assert.Loosely(t, pars[8].Value, should.Equal("fine"))
				assert.Loosely(t, pars[9].Value, should.Equal("case_name"))
				// In LIKE clauses, _ needs to be escaped with `\` to match literal `_`.
				assert.Loosely(t, pars[10].Value, should.Equal(`%:module!gtest::fine#case\_name%`))
			})

			t.Run("module!scheme", func(t *ftt.Test) {
				filter, err := aip160.ParseFilter(`test_id:"mod!sch"`)
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "T", "p_")
				assert.Loosely(t, err, should.BeNil)

				// Should match:
				// Module -> Scheme transition.
				assert.Loosely(t, result, should.Equal(strings.Join([]string{
					`(((ENDS_WITH(T.ModuleName, @p_0) AND STARTS_WITH(T.ModuleScheme, @p_1)) AND T.ModuleName <> "legacy")`,
					` OR (T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_2))`,
				}, "")))
				assert.Loosely(t, pars, should.HaveLength(3))
				assert.Loosely(t, pars[0].Value, should.Equal("mod"))
				assert.Loosely(t, pars[1].Value, should.Equal("sch"))
				assert.Loosely(t, pars[2].Value, should.Equal("%mod!sch%"))
			})
			t.Run(":module or :coarse or :fine or :case_part_2", func(t *ftt.Test) {
				filter, err := aip160.ParseFilter(`test_id:":part"`)
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "T", "p_")
				assert.Loosely(t, err, should.BeNil)

				expectedQuery := strings.Join([]string{
					`(((STARTS_WITH(T.ModuleName, @p_0) OR STARTS_WITH(T.T1CoarseName, @p_1) OR STARTS_WITH(T.T2FineName, @p_2) OR T.T3CaseName LIKE @p_3) AND T.ModuleName <> "legacy") OR `,
					`((T.ModuleName LIKE @p_4 OR T.ModuleScheme LIKE @p_5 OR T.T1CoarseName LIKE @p_6 OR T.T2FineName LIKE @p_7 OR T.T3CaseName LIKE @p_8) AND T.ModuleName <> "legacy") OR `,
					`(T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_9))`,
				}, "")

				assert.Loosely(t, result, should.Equal(expectedQuery))
				assert.Loosely(t, pars, should.HaveLength(10))
				assert.Loosely(t, pars[0].Value, should.Equal("part"))
				assert.Loosely(t, pars[1].Value, should.Equal("part"))
				assert.Loosely(t, pars[2].Value, should.Equal("part"))
				assert.Loosely(t, pars[3].Value, should.Equal("%:part%"))
				assert.Loosely(t, pars[4].Value, should.Equal("%:part%"))
				assert.Loosely(t, pars[5].Value, should.Equal("%:part%"))
				assert.Loosely(t, pars[6].Value, should.Equal("%:part%"))
				assert.Loosely(t, pars[7].Value, should.Equal("%:part%"))
				assert.Loosely(t, pars[8].Value, should.Equal("%\\\\:part%"))
				assert.Loosely(t, pars[9].Value, should.Equal("%:part%"))
			})
			t.Run("scheme:coarse, coarse:fine and case_part_1:case_part_2", func(t *ftt.Test) {
				// Should match three cases:
				// - Scheme -> Coarse.
				// - Coarse -> Fine.
				// - Inside CaseName (extended depth test ID hierarchy).
				// Plus the legacy test ID case.
				expectedQuery := strings.Join([]string{
					`((((ENDS_WITH(T.ModuleScheme, @p_0) AND STARTS_WITH(T.T1CoarseName, @p_1)) OR `,
					`(ENDS_WITH(T.T1CoarseName, @p_2) AND STARTS_WITH(T.T2FineName, @p_3)) OR `,
					`T.T3CaseName LIKE @p_4) AND T.ModuleName <> "legacy") OR `,
					`(T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_5))`,
				}, "")

				t.Run("basic", func(t *ftt.Test) {
					filter, err := aip160.ParseFilter(`test_id:"sch:coa"`)
					assert.Loosely(t, err, should.BeNil)

					result, pars, err := table.WhereClause(filter, "T", "p_")
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, result, should.Equal(expectedQuery))
					assert.Loosely(t, pars, should.HaveLength(6))
					assert.Loosely(t, pars[0].Value, should.Equal("sch"))
					assert.Loosely(t, pars[1].Value, should.Equal("coa"))
					assert.Loosely(t, pars[2].Value, should.Equal("sch"))
					assert.Loosely(t, pars[3].Value, should.Equal("coa"))
					assert.Loosely(t, pars[4].Value, should.Equal("%sch:coa%"))
					assert.Loosely(t, pars[5].Value, should.Equal("%sch:coa%"))
				})
				t.Run("escaping", func(t *ftt.Test) {
					// Input: `a\\a:b\:\#\!`
					// If interpreted as structured test ID, the segments are {value: `a\a`},{separator:':', value: `b:#!`}
					filter, err := aip160.ParseFilter(`test_id:"a\\\\a:b\\:\\#\\!"`)
					assert.Loosely(t, err, should.BeNil)

					result, pars, err := table.WhereClause(filter, "T", "p_")
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, result, should.Equal(expectedQuery))

					assert.Loosely(t, pars, should.HaveLength(6))
					assert.Loosely(t, pars[0].Value, should.Equal(`a\a`))
					assert.Loosely(t, pars[1].Value, should.Equal(`b:#!`))
					assert.Loosely(t, pars[2].Value, should.Equal(`a\a`))
					assert.Loosely(t, pars[3].Value, should.Equal(`b:#!`))
					// To match '\' in a LIKE clause, we need `\\` to indicate we are not in a LIKE escape sequence.
					// When `\` or `:` appears inside a test case name segment, it is encoded with a `\`.
					assert.Loosely(t, pars[4].Value, should.Equal(`%a\\\\a:b\\:#!%`))
					// To match '\' in a LIKE clause, we need `\\` to indicate we are not in a LIKE escape sequence.
					assert.Loosely(t, pars[5].Value, should.Equal(`%a\\\\a:b\\:\\#\\!%`)) // Legacy param
				})
			})
			t.Run("fine#case", func(t *ftt.Test) {
				filter, err := aip160.ParseFilter(`test_id:"fine#case"`)
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "T", "p_")
				assert.Loosely(t, err, should.BeNil)

				// Should match two cases:
				// - Fine -> Case.
				// - As legacy test ID.
				assert.Loosely(t, result, should.Equal(strings.Join([]string{
					`(((ENDS_WITH(T.T2FineName, @p_0) AND STARTS_WITH(T.T3CaseName, @p_1)) AND T.ModuleName <> "legacy") OR `,
					`(T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_2))`,
				}, "")))
				assert.Loosely(t, pars, should.HaveLength(3))
				assert.Loosely(t, pars[0].Value, should.Equal("fine"))
				assert.Loosely(t, pars[1].Value, should.Equal("case"))
				assert.Loosely(t, pars[2].Value, should.Equal("%fine#case%"))
			})

			t.Run("!scheme", func(t *ftt.Test) {
				filter, err := aip160.ParseFilter(`test_id:"!sch"`)
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "T", "p_")
				assert.Loosely(t, err, should.BeNil)

				// This reflects the following options analysis:
				// - if ! is grammar, it can only match in one place: the module scheme.
				// - if ! is content because the input string truncated the previous escape '\' character, ! could appear in any test ID component.
				//   (technically anywhere but the scheme due to alphabetic restrictions but the implementation does not have this reasoning ability).
				// - it could appear in a legacy test ID.
				assert.Loosely(t, result, should.Equal(strings.Join([]string{
					`((STARTS_WITH(T.ModuleScheme, @p_0) AND T.ModuleName <> "legacy") OR `,
					`((T.ModuleName LIKE @p_1 OR T.ModuleScheme LIKE @p_2 OR T.T1CoarseName LIKE @p_3 OR T.T2FineName LIKE @p_4 OR T.T3CaseName LIKE @p_5) AND T.ModuleName <> "legacy") OR `,
					`(T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_6))`,
				}, "")))
				assert.Loosely(t, pars, should.HaveLength(7))
				assert.Loosely(t, pars[0].Value, should.Equal("sch"))
				assert.Loosely(t, pars[1].Value, should.Equal("%!sch%"))
				assert.Loosely(t, pars[2].Value, should.Equal("%!sch%"))
				assert.Loosely(t, pars[3].Value, should.Equal("%!sch%"))
				assert.Loosely(t, pars[4].Value, should.Equal("%!sch%"))
				assert.Loosely(t, pars[5].Value, should.Equal("%!sch%"))
				assert.Loosely(t, pars[6].Value, should.Equal("%!sch%"))
			})

			t.Run("invalid as structured ID", func(t *ftt.Test) {
				// Input: foo\bar
				// \b is invalid escape in structured ID.
				// Should fallback to legacy match only.
				filter, err := aip160.ParseFilter(`test_id:"foo\\bar"`)
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "T", "p_")
				assert.Loosely(t, err, should.BeNil)

				// Should NOT contain structured match parts (e.g. OR ...).
				// Should only contain legacy match.
				assert.Loosely(t, result, should.Equal(`((T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_0))`))

				// Verify parameter is "foo\bar"
				assert.Loosely(t, pars, should.HaveLength(1))
				assert.Loosely(t, pars[0].Value, should.Equal("%foo\\\\bar%"))
			})
			t.Run("trailing escape sequence truncated", func(t *ftt.Test) {
				// Consider a real flat ID had text like `:some\\value!scheme:coarse:fine#case` or
				// `:some\:value!scheme:coarse:fine#case` and we search for `:some\`.
				// In AIP-160 syntax, '\' needs to be escaped as "\\".
				filter, err := aip160.ParseFilter(`test_id:":some\\"`)
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "T", "p_")
				assert.Loosely(t, err, should.BeNil)

				// Should NOT contain structured match parts (e.g. OR ...).
				// Should only contain legacy match.
				assert.Loosely(t, result, should.Equal(strings.Join([]string{
					`(((REGEXP_CONTAINS(T.ModuleName, @p_0) OR REGEXP_CONTAINS(T.T1CoarseName, @p_1) OR REGEXP_CONTAINS(T.T2FineName, @p_2) OR REGEXP_CONTAINS(T.T3CaseName, @p_3)) AND T.ModuleName <> "legacy") OR `,
					`((REGEXP_CONTAINS(T.ModuleName, @p_4) OR REGEXP_CONTAINS(T.ModuleScheme, @p_5) OR REGEXP_CONTAINS(T.T1CoarseName, @p_6) OR REGEXP_CONTAINS(T.T2FineName, @p_7) OR REGEXP_CONTAINS(T.T3CaseName, @p_8)) AND T.ModuleName <> "legacy") OR `,
					`(T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_9))`,
				}, "")))
				assert.Loosely(t, pars, should.HaveLength(10))
				// For case where leading ':' matches grammar.
				assert.Loosely(t, pars[0].Value, should.Equal(`^some[:!#\\]`))
				assert.Loosely(t, pars[1].Value, should.Equal(`^some[:!#\\]`))
				assert.Loosely(t, pars[2].Value, should.Equal(`^some[:!#\\]`))
				// In case names, : and \ are escaped if they appear in one of the extended hierarchy components.
				assert.Loosely(t, pars[3].Value, should.Equal(`:some([#!]|\\[:\\])`))
				// For case where leading escape '\' was truncated, e.g. real test ID like "module\:some\\value!scheme:coarse:fine#case".
				assert.Loosely(t, pars[4].Value, should.Equal(`:some[:!#\\]`))
				assert.Loosely(t, pars[5].Value, should.Equal(`:some[:!#\\]`))
				assert.Loosely(t, pars[6].Value, should.Equal(`:some[:!#\\]`))
				assert.Loosely(t, pars[7].Value, should.Equal(`:some[:!#\\]`))
				// In case names, : and \ are escaped if they appear in one of the extended hierarchy components.
				assert.Loosely(t, pars[8].Value, should.Equal(`\\:some([#!]|\\[:\\])`))
				assert.Loosely(t, pars[9].Value, should.Equal(`%:some\\%`))
			})
			t.Run("leading escape sequence truncated", func(t *ftt.Test) {
				// Consider a real flat ID had text like `:a\\value!module:coarse:fine#case`
				// where the search string is just `\value`.
				// In AIP-160 syntax, '\' needs to be escaped as "\\".
				filter, err := aip160.ParseFilter(`test_id:"\\value"`)
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "T", "p_")
				assert.Loosely(t, err, should.BeNil)

				// "\value" is not valid as a partial structured test ID parsed in the normal way
				// as \v is not a valid escape sequence.
				// The only way it could be valid is if a preceding "\" was truncated and
				// it actually contained the longer substring "\\value".
				assert.Loosely(t, result, should.Equal(strings.Join([]string{
					`(((T.ModuleName LIKE @p_0 OR T.ModuleScheme LIKE @p_1 OR T.T1CoarseName LIKE @p_2 OR T.T2FineName LIKE @p_3 OR T.T3CaseName LIKE @p_4) AND T.ModuleName <> "legacy") OR `,
					`(T.ModuleName = "legacy" AND T.ModuleScheme = "legacy" AND T.T1CoarseName = "" AND T.T2FineName = "" AND T.T3CaseName LIKE @p_5))`,
				}, "")))
				assert.Loosely(t, pars, should.HaveLength(6))
				assert.Loosely(t, pars[0].Value, should.Equal(`%\\value%`))
				assert.Loosely(t, pars[1].Value, should.Equal(`%\\value%`))
				assert.Loosely(t, pars[2].Value, should.Equal(`%\\value%`))
				assert.Loosely(t, pars[3].Value, should.Equal(`%\\value%`))
				// \ is escaped as \\ in case names. To match \ in a like clause, it needs
				// to be escaped as \\.
				assert.Loosely(t, pars[4].Value, should.Equal(`%\\\\value%`))
				assert.Loosely(t, pars[5].Value, should.Equal(`%\\value%`))
			})
		})
	})
}

func TestTestIDFilterIntegration(t *testing.T) {
	ftt.Run("TestTestIDFilterIntegration", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		// Create the root invocation and shards.
		ri := rootinvocations.NewBuilder("root-inv-id").WithRealm("testproject:testrealm").Build()
		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(ri)...)
		rootInvShard := rootinvocations.ShardID{
			RootInvocationID: "root-inv-id",
			ShardIndex:       1,
		}

		testID := pbutil.BaseTestIdentifier{
				ModuleName:   "a",
				ModuleScheme: "s",
				CoarseName:   ":x",
				FineName:     "!v",
				CaseName:     `\:y\\:\:`,
		}
		flatTestID := pbutil.EncodeTestID(testID)

		tr := NewBuilder().
			WithRootInvocationShardID(rootInvShard).
			WithResultID("result-1").
			WithModuleName(testID.ModuleName).
			WithModuleScheme(testID.ModuleScheme).
			WithCoarseName(testID.CoarseName).
			WithFineName(testID.FineName).
			WithCaseName(testID.CaseName).
			Build()

		testutil.MustApply(ctx, t, Create(tr))

		fieldNames := StructuredTestIDColumnNames{
			ModuleName:   "ModuleName",
			ModuleScheme: "ModuleScheme",
			CoarseName:   "T1CoarseName",
			FineName:     "T2FineName",
			CaseName:     "T3CaseName",
		}

		table := aip160.NewDatabaseTable().WithFields(
			aip160.NewField().WithFieldPath("test_id").WithBackend(
				NewFlatTestIDFieldBackend(fieldNames),
			).Filterable().Build(),
		).Build()

		test := func (substring string, expectMatch bool) {
			filterStr := fmt.Sprintf("test_id:%q", substring)
			filter, err := aip160.ParseFilter(filterStr)
			assert.Loosely(t, err, should.BeNil, truth.Explain("substring %q", substring))

			whereClause, args, err := table.WhereClause(filter, "TestResultsV2", "p_")
			assert.Loosely(t, err, should.BeNil, truth.Explain("substring %q", substring))

			// Execute the query.
			stmt := spanner.Statement{
				SQL: fmt.Sprintf(`
					SELECT RootInvocationShardId
					FROM TestResultsV2
					WHERE %s
				`, whereClause),
				Params: make(map[string]interface{}),
			}
			for _, arg := range args {
				stmt.Params[arg.Name] = arg.Value
			}

			var gotShardIDStr string
			rowLen := 0
			iter := span.Query(span.Single(ctx), stmt)
			err = iter.Do(func(r *spanner.Row) error {
				rowLen++
				return r.ColumnByName("RootInvocationShardId", &gotShardIDStr)
			})
			assert.Loosely(t, err, should.BeNil, truth.Explain("substring %q", substring))

			// Verify we found the result.
			if expectMatch {
				assert.Loosely(t, rowLen, should.Equal(1), truth.Explain("substring %q", substring))
				assert.Loosely(t, gotShardIDStr, should.Equal(rootInvShard.RowID()), truth.Explain("substring %q", substring))
			} else {
				assert.Loosely(t, rowLen, should.Equal(0), truth.Explain("substring %q", substring))
			}
		}

		// Iterate through every substring of the flat test ID.
		for i := 0; i < len(flatTestID); i++ {
			for j := i + 1; j <= len(flatTestID); j++ {
				substring := flatTestID[i:j]
				test(substring, true)

				// "f" does not appear in the test ID, so extending the substring by this character
				// should lead to a failure to match.
				test(substring + "f", false)
			}
		}
	})
}
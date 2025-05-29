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

// Package rules provides methods to evaluate test name clustering rules.
package rules

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/clustering/rules/lang"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// likeRewriter escapes usages of '\', '%' and '_', so that
// the original text is interpreted literally in a LIKE
// expression.
var likeRewriter = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// substitutionRE matches use of the '$' operator which
// may be used in templates to substitute in values.
// Captured usages are either:
//   - ${name}, which tells the template to insert the value
//     of the capture group with that name.
//   - $$, which tells the template insert a literal '$'
//     into the output.
//   - $, which indicates an invalid use of the '$' operator
//     ($ not followed by $ or {name}).
var substitutionRE = regexp.MustCompile(`\$\{(\w+?)\}|\$\$?`)

// Evaluator evaluates a test name clustering rule on
// a test name, returning whether the rule matches and
// if so, the LIKE expression that defines the cluster.
type Evaluator func(testName string) (like string, ok bool)

// Compile produces a RuleEvaluator that can quickly evaluate
// whether a given test name matches the given test name
// clustering rule, and if so, return the test name LIKE
// expression that defines the cluster.
//
// As Compiling rules is slow, the result should be cached.
func Compile(rule *configpb.TestNameClusteringRule) (Evaluator, error) {
	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return nil, errors.Fmt("pattern: %w", err)
	}

	// Segments defines portions of the output LIKE expression,
	// which are either literal text found in the LikeTemplate,
	// or parts of the test name matched by Pattern.
	var segments []segment

	// The exclusive upper bound we have created segments for.
	lastIndex := 0

	// Analyze the specified LikeTemplate to identify the
	// location of all substitution expressions (of the form ${name})
	// and iterate through them.
	matches := substitutionRE.FindAllStringSubmatchIndex(rule.LikeTemplate, -1)
	for _, match := range matches {
		// The start and end of the substitution expression (of the form ${name})
		// in c.LikeTemplate.
		matchStart := match[0]
		matchEnd := match[1]

		if matchStart > lastIndex {
			// There is some literal text between the start of the LikeTemplate
			// and the first substitution expression, or the last substitution
			// expression and the current one. This is literal
			// text that should be included in the output directly.
			literalText := rule.LikeTemplate[lastIndex:matchStart]
			if err := lang.ValidateLikePattern(literalText); err != nil {
				return nil, errors.Fmt("%q is not a valid standalone LIKE expression: %w", literalText, err)
			}
			segments = append(segments, &literalSegment{
				value: literalText,
			})
		}

		matchString := rule.LikeTemplate[match[0]:match[1]]
		if matchString == "$" {
			return nil, fmt.Errorf("like_template: invalid use of the $ operator at position %v in %q ('$' not followed by '{name}' or '$'), "+
				"if you meant to include a literal $ character, please use $$", match[0], rule.LikeTemplate)
		}
		if matchString == "$$" {
			// Insert the literal "$" into the output.
			segments = append(segments, &literalSegment{
				value: "$",
			})
		} else {
			// The name of the capture group that should be substituted at
			// the current position.
			name := rule.LikeTemplate[match[2]:match[3]]

			// Find the index of the corresponding capture group in the
			// Pattern.
			submatchIndex := -1
			for i, submatchName := range re.SubexpNames() {
				if submatchName == "" {
					// Unnamed capturing groups can not be referred to.
					continue
				}
				if submatchName == name {
					submatchIndex = i
					break
				}
			}
			if submatchIndex == -1 {
				return nil, fmt.Errorf("like_template: contains reference to non-existant capturing group with name %q", name)
			}

			// Indicate we should include the value of that capture group
			// in the output.
			segments = append(segments, &submatchSegment{
				submatchIndex: submatchIndex,
			})
		}
		lastIndex = matchEnd
	}

	if lastIndex < len(rule.LikeTemplate) {
		literalText := rule.LikeTemplate[lastIndex:len(rule.LikeTemplate)]
		if err := lang.ValidateLikePattern(literalText); err != nil {
			return nil, errors.Fmt("like_template: %q is not a valid standalone LIKE expression: %w", literalText, err)
		}
		// Some text after all substitution expressions. This is literal
		// text that should be included in the output directly.
		segments = append(segments, &literalSegment{
			value: literalText,
		})
	}

	// Produce the evaluator. This is in the hot-path that is run
	// on every test result on every ingestion or config change,
	// so it should be fast. We do not want to be parsing regular
	// expressions or templates in here.
	evaluator := func(testName string) (like string, ok bool) {
		m := re.FindStringSubmatch(testName)
		if m == nil {
			return "", false
		}
		segmentValues := make([]string, len(segments))
		for i, s := range segments {
			segmentValues[i] = s.evaluate(m)
		}
		return strings.Join(segmentValues, ""), true
	}
	return evaluator, nil
}

// literalSegment is a part of a constructed string
// that is a constant string value.
type literalSegment struct {
	// The literal value that defines this segment.
	value string
}

func (c *literalSegment) evaluate(matches []string) string {
	return c.value
}

// submatchSegment is a part of a constructed string
// that is populated with a matched portion of
// another source string.
type submatchSegment struct {
	// The source string submatch index that defines this segment.
	submatchIndex int
}

func (m *submatchSegment) evaluate(matches []string) string {
	return likeRewriter.Replace(matches[m.submatchIndex])
}

// segment represents a part of a constructed string.
type segment interface {
	evaluate(matches []string) string
}

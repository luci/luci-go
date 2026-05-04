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

import { astToString } from '../ast/ast';

import { parseFilter } from './parser';

describe('AIP-160 Filter Parser', () => {
  const testCases: readonly {
    input: string;
    ast?: string;
    expectErr?: boolean;
  }[] = [
    { input: '', ast: 'filter{}' },
    { input: ' ', ast: 'filter{}' },
    {
      input: 'simple',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{"simple"}}}}}}}}}',
    },
    {
      input: ' wsBefore',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{"wsBefore"}}}}}}}}}',
    },
    {
      input: 'wsAfter ',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{"wsAfter"}}}}}}}}}',
    },
    {
      input: ' wsAround ',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{"wsAround"}}}}}}}}}',
    },
    {
      input: '"string"',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{quoted,"string"}}}}}}}}}',
    },
    {
      input: ' "string" ',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{quoted,"string"}}}}}}}}}',
    },
    {
      input: '"ws string"',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{quoted,"ws string"}}}}}}}}}',
    },
    {
      input: '-negated',
      ast: 'filter{expression{sequence{factor{term{-restriction{comparable{member{value{"negated"}}}}}}}}}',
    },
    {
      input: ' - negated ',
      ast: 'filter{expression{sequence{factor{term{-restriction{comparable{member{value{"negated"}}}}}}}}}',
    },
    {
      input: 'dash-separated-name',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{"dash-separated-name"}}}}}}}}}',
    },
    {
      input: 'term -negated-term',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"term"}}}}}},' +
        'factor{term{-restriction{comparable{member{value{"negated-term"}}}}}}}}}',
    },
    {
      input: 'NOT negated',
      ast: 'filter{expression{sequence{factor{term{-restriction{comparable{member{value{"negated"}}}}}}}}}',
    },
    {
      input: ' NOT negated ',
      ast: 'filter{expression{sequence{factor{term{-restriction{comparable{member{value{"negated"}}}}}}}}}',
    },
    {
      input: ' NOTnegated ',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{"NOTnegated"}}}}}}}}}',
    },
    {
      input: 'implicit and',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"implicit"}}}}}},' +
        'factor{term{restriction{comparable{member{value{"and"}}}}}}}}}',
    },
    {
      input: ' implicit and ',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"implicit"}}}}}},' +
        'factor{term{restriction{comparable{member{value{"and"}}}}}}}}}',
    },
    {
      input: 'explicit AND and',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"explicit"}}}}}}},' +
        'sequence{factor{term{restriction{comparable{member{value{"and"}}}}}}}}}',
    },
    { input: 'explicit AND ', expectErr: true },
    {
      input: 'explicit AND and',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"explicit"}}}}}}},' +
        'sequence{factor{term{restriction{comparable{member{value{"and"}}}}}}}}}',
    },
    {
      input: ' explicit AND and ',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"explicit"}}}}}}},' +
        'sequence{factor{term{restriction{comparable{member{value{"and"}}}}}}}}}',
    },
    {
      input: ' explicit ANDnotand ',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"explicit"}}}}}},' +
        'factor{term{restriction{comparable{member{value{"ANDnotand"}}}}}}}}}',
    },
    {
      input: 'test OR or',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"test"}}}}},' +
        'term{restriction{comparable{member{value{"or"}}}}}}}}}',
    },
    { input: 'test OR ', expectErr: true },
    {
      input: 'test ORnotor',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"test"}}}}}},' +
        'factor{term{restriction{comparable{member{value{"ORnotor"}}}}}}}}}',
    },
    {
      input: ' test OR or ',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"test"}}}}},' +
        'term{restriction{comparable{member{value{"or"}}}}}}}}}',
    },
    {
      input: ' testORor ',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{"testORor"}}}}}}}}}',
    },
    {
      input: 'implicit and AND explicit',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"implicit"}}}}}},' +
        'factor{term{restriction{comparable{member{value{"and"}}}}}}},' +
        'sequence{factor{term{restriction{comparable{member{value{"explicit"}}}}}}}}}',
    },
    {
      input: 'implicit with OR term AND explicit OR term',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"implicit"}}}}}},' +
        'factor{term{restriction{comparable{member{value{"with"}}}}},' +
        'term{restriction{comparable{member{value{"term"}}}}}}},' +
        'sequence{factor{term{restriction{comparable{member{value{"explicit"}}}}},' +
        'term{restriction{comparable{member{value{"term"}}}}}}}}}',
    },
    {
      input: '(composite)',
      ast:
        'filter{expression{sequence{factor{term{expression{sequence{' +
        'factor{term{restriction{comparable{member{value{"composite"}}}}}}}}}}}}}',
    },
    {
      input: ' (composite) ',
      ast:
        'filter{expression{sequence{factor{term{expression{sequence{' +
        'factor{term{restriction{comparable{member{value{"composite"}}}}}}}}}}}}}',
    },
    {
      input: '( composite )',
      ast:
        'filter{expression{sequence{factor{term{expression{sequence{' +
        'factor{term{restriction{comparable{member{value{"composite"}}}}}}}}}}}}}',
    },
    {
      input: ' ( composite ) ',
      ast:
        'filter{expression{sequence{factor{term{expression{sequence{' +
        'factor{term{restriction{comparable{member{value{"composite"}}}}}}}}}}}}}',
    },
    {
      input: ' ( composite multi) ',
      ast:
        'filter{expression{sequence{factor{term{expression{sequence{' +
        'factor{term{restriction{comparable{member{value{"composite"}}}}}},' +
        'factor{term{restriction{comparable{member{value{"multi"}}}}}}}}}}}}}',
    },
    {
      input: 'value<21',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},"<",' +
        'comparable{member{value{"21"}}}}}}}}}',
    },
    {
      input: 'value < 21',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},"<",' +
        'comparable{member{value{"21"}}}}}}}}}',
    },
    {
      input: ' value < 21 ',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},"<",' +
        'comparable{member{value{"21"}}}}}}}}}',
    },
    {
      input: 'value<=21',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},"<=",' +
        'comparable{member{value{"21"}}}}}}}}}',
    },
    {
      input: 'value>21',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},">",' +
        'comparable{member{value{"21"}}}}}}}}}',
    },
    {
      input: 'value>=21',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},">=",' +
        'comparable{member{value{"21"}}}}}}}}}',
    },
    {
      input: 'value=21',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},"=",' +
        'comparable{member{value{"21"}}}}}}}}}',
    },
    {
      input: 'value!=21',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},"!=",' +
        'comparable{member{value{"21"}}}}}}}}}',
    },
    {
      input: 'value:21',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},":",' +
        'comparable{member{value{"21"}}}}}}}}}',
    },
    {
      input: 'value=(composite)',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"value"}}},"=",expression{sequence{' +
        'factor{term{restriction{comparable{member{value{"composite"}}}}}}}}}}}}}}',
    },
    {
      input: 'member.field',
      ast: 'filter{expression{sequence{factor{term{restriction{comparable{member{value{"member"}, {value{"field"}}}}}}}}}}',
    },
    {
      input: ' member.field > 4 ',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"member"}, {value{"field"}}}},">",' +
        'comparable{member{value{"4"}}}}}}}}}',
    },
    {
      input: ' member."field" > 4 ',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"member"}, {value{quoted,"field"}}}},">",' +
        'comparable{member{value{"4"}}}}}}}}}',
    },
    {
      input: 'composite (expression)',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"composite"}}}}}},' +
        'factor{term{expression{sequence{' +
        'factor{term{restriction{comparable{member{value{"expression"}}}}}}}}}}}}}',
    },
    {
      input:
        'labels."dut_state" = ("needs_manual_repair" OR "needs_repair") ' +
        'labels."label-pool" = "aft-satlab" ' +
        '(NOT labels."bot_id" OR labels."bot_id" = "cloudbots-canary-e2-custom-2-6144-c8-r1-r1-h9-bjcq") ' +
        'NOT host',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"labels"}, {value{quoted,"dut_state"}}}},' +
        '"=",expression{sequence{factor{term{restriction{comparable{member{value{quoted,"needs_manual_repair"}}}}},term{' +
        'restriction{comparable{member{value{quoted,"needs_repair"}}}}}}}}}}},factor{term{restriction{comparable{member' +
        '{value{"labels"}, {value{quoted,"label-pool"}}}},"=",comparable{member{value{quoted,"aft-satlab"}}}}}},factor{term{' +
        'expression{sequence{factor{term{-restriction{comparable{member{value{"labels"}, {value{quoted,"bot_id"}}}}}},term{' +
        'restriction{comparable{member{value{"labels"}, {value{quoted,"bot_id"}}}},"=",comparable{member{value{' +
        'quoted,"cloudbots-canary-e2-custom-2-6144-c8-r1-r1-h9-bjcq"}}}}}}}}}},factor{term{-restriction{comparable{member' +
        '{value{"host"}}}}}}}}}',
    },
    {
      input: `NOT labels."allocation_key" labels."allocation_key" = (dirtyflash_reserve__git_main__14672933__eos-trunk_staging-userdebug)`,
      ast:
        'filter{expression{sequence{factor{term{-restriction{comparable{member{value{"labels"}, {value{quoted,"allocation_key"}}}}}}}' +
        ',factor{term{restriction{comparable{member{value{"labels"}, {value{quoted,"allocation_key"}}}},"=",expression{sequence{factor{' +
        'term{restriction{comparable{member{value{"dirtyflash_reserve__git_main__14672933__eos-trunk_staging-userdebug"}}}}}}}}}}}}}}',
    },
    {
      input:
        `resource_details=("[Resource Request][Annual] BR-25-16: Falcon RAK w/NVIDIA RTX 5070 x 6" OR` +
        ` "[Resource Request][Annual] BR-25-14 & BR-25-16: Falcon RAK w/ NVIDIA RTX 5070 x 6")`,
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"resource_details"}}},"=",' +
        'expression{sequence{factor{term{restriction{comparable{member{' +
        'value{quoted,"[Resource Request][Annual] BR-25-16: Falcon RAK w/NVIDIA RTX 5070 x 6"}}}}},' +
        'term{restriction{comparable{member{' +
        'value{quoted,"[Resource Request][Annual] BR-25-14 & BR-25-16: Falcon RAK w/ NVIDIA RTX 5070 x 6"}}}}}}}}}}}}}}',
    },
    {
      input: 'label != "A" AND label != "B"',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"label"}}},"!=",' +
        'comparable{member{value{quoted,"A"}}}}}}},' +
        'sequence{factor{term{restriction{comparable{member{value{"label"}}},"!=",comparable{member{value{quoted,"B"}}}}}}}}}',
    },
    {
      input: 'NOT key:*',
      ast: 'filter{expression{sequence{factor{term{-restriction{comparable{member{value{"key"}}},":",comparable{member{value{"*"}}}}}}}}}',
    },
    {
      input: 'key = "v1" OR key = "v2"',
      ast:
        'filter{expression{sequence{factor{term{restriction{comparable{member{value{"key"}}},"=",' +
        'comparable{member{value{quoted,"v1"}}}}},' +
        'term{restriction{comparable{member{value{"key"}}},"=",comparable{member{value{quoted,"v2"}}}}}}}}}',
    },
  ];

  it.each(testCases.filter((tc) => !tc.expectErr))(
    'should parse "$input"',
    ({ input, ast }) => {
      const result = parseFilter(input);
      expect(result.isError).toBe(false);
      if (result.isError) {
        throw new Error(`Unexpected parse error: ${result.error}`);
      }
      expect(astToString(result.ast)).toBe(ast);
    },
  );

  it.each(testCases.filter((tc) => tc.expectErr))(
    'should fail to parse "$input"',
    ({ input }) => {
      const result = parseFilter(input);
      expect(result.isError).toBe(true);
    },
  );
});

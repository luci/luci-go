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

import { Filter, astToString } from './ast';

describe('AST', () => {
  // The AST here represents: NOT a.b: "foo" AND (c < d OR NOT e)
  it('astToString', () => {
    const filter: Filter = {
      kind: 'Filter',
      expression: {
        kind: 'Expression',
        sequences: [
          {
            kind: 'Sequence',
            factors: [
              {
                kind: 'Factor',
                terms: [
                  {
                    kind: 'Term',
                    negated: true,
                    simple: {
                      kind: 'Restriction',
                      comparable: {
                        kind: 'Comparable',
                        member: {
                          kind: 'Member',
                          value: { kind: 'Value', quoted: false, value: 'a' },
                          fields: [
                            { kind: 'Value', quoted: false, value: 'b' },
                          ],
                        },
                      },
                      comparator: ':',
                      arg: {
                        kind: 'Comparable',
                        member: {
                          kind: 'Member',
                          value: { kind: 'Value', quoted: true, value: 'foo' },
                          fields: [],
                        },
                      },
                    },
                  },
                ],
              },
            ],
          },
          {
            kind: 'Sequence',
            factors: [
              {
                kind: 'Factor',
                terms: [
                  {
                    kind: 'Term',
                    negated: false,
                    simple: {
                      kind: 'Expression',
                      sequences: [
                        {
                          kind: 'Sequence',
                          factors: [
                            {
                              kind: 'Factor',
                              terms: [
                                // c < d
                                {
                                  kind: 'Term',
                                  negated: false,
                                  simple: {
                                    kind: 'Restriction',
                                    comparable: {
                                      kind: 'Comparable',
                                      member: {
                                        kind: 'Member',
                                        value: {
                                          kind: 'Value',
                                          quoted: false,
                                          value: 'c',
                                        },
                                        fields: [],
                                      },
                                    },
                                    comparator: '<',
                                    arg: {
                                      kind: 'Comparable',
                                      member: {
                                        kind: 'Member',
                                        value: {
                                          kind: 'Value',
                                          quoted: false,
                                          value: 'd',
                                        },
                                        fields: [],
                                      },
                                    },
                                  },
                                },
                                // OR NOT e
                                {
                                  kind: 'Term',
                                  negated: true,
                                  simple: {
                                    kind: 'Restriction',
                                    comparable: {
                                      kind: 'Comparable',
                                      member: {
                                        kind: 'Member',
                                        value: {
                                          kind: 'Value',
                                          quoted: false,
                                          value: 'e',
                                        },
                                        fields: [],
                                      },
                                    },
                                    comparator: '',
                                    arg: null,
                                  },
                                },
                              ],
                            },
                          ],
                        },
                      ],
                    },
                  },
                ],
              },
            ],
          },
        ],
      },
    };

    const expected =
      'filter{expression{sequence{factor{term{-restriction{comparable{member{value{"a"}, {value{"b"}}}},":",' +
      'comparable{member{value{quoted,"foo"}}}}}}},sequence{factor{term{expression{sequence{factor{term{' +
      'restriction{comparable{member{value{"c"}}},"<",comparable{member{value{"d"}}}}},term{-' +
      'restriction{comparable{member{value{"e"}}}}}}}}}}}}}';

    expect(astToString(filter)).toBe(expected);
  });
});

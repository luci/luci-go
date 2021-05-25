// Copyright 2021 The LUCI Authors.
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

import * as chai from 'chai';
import { assert } from 'chai';

import { chaiDeepIncludeProperties } from '../../libs/test_utils/chai_deep_include_properties';
import { StepExt } from '../../models/step_ext';
import { traverseStepList } from './timeline_tab';

chai.use(chaiDeepIncludeProperties);

const rootSteps = [
  {
    name: 'root1',
    selfName: 'root1',
    depth: 0,
    index: 0,
    children: [] as StepExt[],
  },
  {
    name: 'root2',
    selfName: 'root2',
    depth: 0,
    index: 1,
    children: [
      {
        name: 'root2|parent1',
        selfName: 'parent1',
        depth: 1,
        index: 0,
        children: [
          {
            name: 'root2|parent1|child1',
            selfName: 'child1',
            depth: 2,
            index: 0,
            children: [],
          },
          {
            name: 'root2|parent1|child2',
            selfName: 'child2',
            depth: 2,
            index: 1,
            children: [],
          },
        ],
      },
    ],
  },
  {
    name: 'root3',
    selfName: 'root3',
    depth: 0,
    index: 2,
    children: [
      {
        name: 'root3|parent1',
        selfName: 'parent1',
        depth: 1,
        index: 0,
        children: [],
      },
      {
        name: 'root3|parent2',
        selfName: 'parent2',
        depth: 1,
        index: 1,
        children: [
          {
            name: 'root3|parent2|child1',
            selfName: 'child1',
            depth: 2,
            index: 0,
            children: [],
          },
          {
            name: 'root3|parent2|child2',
            selfName: 'child2',
            depth: 2,
            index: 1,
            children: [],
          },
        ],
      },
    ],
  },
] as StepExt[];

describe('traverseStepList', () => {
  it('can traverse step list correctly', () => {
    const stepList = [...traverseStepList(rootSteps)];
    assert.deepIncludeProperties(stepList, [
      [
        '1.',
        {
          name: 'root1',
          selfName: 'root1',
          depth: 0,
          index: 0,
        },
      ],
      [
        '2.',
        {
          name: 'root2',
          selfName: 'root2',
          depth: 0,
          index: 1,
        },
      ],
      [
        '2.1.',
        {
          name: 'root2|parent1',
          selfName: 'parent1',
          depth: 1,
          index: 0,
        },
      ],
      [
        '2.1.1.',
        {
          name: 'root2|parent1|child1',
          selfName: 'child1',
          depth: 2,
          index: 0,
        },
      ],
      [
        '2.1.2.',
        {
          name: 'root2|parent1|child2',
          selfName: 'child2',
          depth: 2,
          index: 1,
        },
      ],
      [
        '3.',
        {
          name: 'root3',
          selfName: 'root3',
          depth: 0,
          index: 2,
        },
      ],
      [
        '3.1.',
        {
          name: 'root3|parent1',
          selfName: 'parent1',
          depth: 1,
          index: 0,
        },
      ],
      [
        '3.2.',
        {
          name: 'root3|parent2',
          selfName: 'parent2',
          depth: 1,
          index: 1,
        },
      ],
      [
        '3.2.1.',
        {
          name: 'root3|parent2|child1',
          selfName: 'child1',
          depth: 2,
          index: 0,
        },
      ],
      [
        '3.2.2.',
        {
          name: 'root3|parent2|child2',
          selfName: 'child2',
          depth: 2,
          index: 1,
        },
      ],
    ] as [string, StepExt][]);
  });
});

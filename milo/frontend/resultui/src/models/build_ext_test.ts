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

import { assert } from 'chai';

import { chaiRecursiveDeepInclude } from '../libs/test_utils/chai_recursive_deep_include';
import { Build, Step } from '../services/buildbucket';
import { BuildExt } from './build_ext';
import { StepExt } from './step_ext';

chai.use(chaiRecursiveDeepInclude);

describe('BuildExt', () => {
  it('should build step-tree correctly', async () => {
    const time = '2020-11-01T21:43:03.351951Z';
    const build = new BuildExt({
      steps: [
        {name: 'root1', startTime: time} as Step,
        {name: 'root2', startTime: time},
        {name: 'root2|parent1', startTime: time},
        {name: 'root2|parent1|child1', startTime: time},
        {name: 'root2|parent1|child2', startTime: time},
        {name: 'root3', startTime: time},
        {name: 'root3|parent1', startTime: time},
        {name: 'root3|parent2', startTime: time},
        {name: 'root3|parent2|child1', startTime: time},
        {name: 'root3|parent2|child2', startTime: time},
      ] as readonly Step[],
    } as Build);

    assert.recursiveDeepInclude(build.rootSteps, [
      {
        name: 'root1',
        children: [] as StepExt[],
      },
      {
        name: 'root2',
        children: [
          {
            name: 'root2|parent1',
            children: [
              {
                name: 'root2|parent1|child1',
                children: [],
              },
              {
                name: 'root2|parent1|child2',
                children: [],
              },
            ],
          },
        ],
      },
      {
        name: 'root3',
        children: [
          {
            name: 'root3|parent1',
            children: [],
          },
          {
            name: 'root3|parent2',
            children: [
              {
                name: 'root3|parent2|child1',
                children: [],
              },
              {
                name: 'root3|parent2|child2',
                children: [],
              },
            ],
          },
        ],
      },
    ] as StepExt[]);
  });
});

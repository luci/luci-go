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

import { TestNode } from './test_node';


describe('Test Node', () => {
  it('should collapse test id segments with no branches', async () => {
    const node = TestNode.newRoot();
    node.addTest({id: 'a/b/c/d', variants: []});
    node.addTest({id: 'a/b/c/e', variants: []});
    node.processTests();
    assert.equal(node.branchName, 'a/b/c/');
    assert.equal(node.children.length, 2);
  });

  it('should un-collapse test id segments after a new branch is added', async () => {
    const node = TestNode.newRoot();
    node.addTest({id: 'a/b/c/d', variants: []});
    node.addTest({id: 'a/b/c/e', variants: []});
    node.processTests();
    assert.equal(node.branchName, 'a/b/c/');
    assert.equal(node.children.length, 2);
    node.addTest({id: 'a/b/f', variants: []});
    node.processTests();
    assert.equal(node.branchName, 'a/b/');
  });
});

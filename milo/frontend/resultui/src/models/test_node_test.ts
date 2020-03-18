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


describe('TestNode', () => {
  it('should elide test id segments with no branches', async () => {
    const node = TestNode.newRoot();
    const test1 = {id: 'a/b/c/d', variants: []};
    const test2 = {id: 'a/b/c/e', variants: []};
    node.addTest(test1);
    node.addTest(test2);
    node.branchUnbranchedTests();
    assert.equal(node.branchName, 'a/b/c/');
    assert.equal(node.children.length, 2);
    assert.equal(node.children[0].branchName, 'd');
    assert.equal(node.children[0].allTests[0], test1);
    assert.equal(node.children[1].branchName, 'e');
    assert.equal(node.children[1].allTests[0], test2);
  });

  it('should un-elide test id segments after a new branch is added', async () => {
    const node = TestNode.newRoot();
    const test1 = {id: 'a/b/c/d', variants: []};
    const test2 = {id: 'a/b/c/e', variants: []};
    node.addTest(test1);
    node.addTest(test2);
    node.branchUnbranchedTests();
    assert.equal(node.branchName, 'a/b/c/');
    assert.equal(node.children.length, 2);

    const test3 = {id: 'a/b/f', variants: []};
    node.addTest(test3);
    node.branchUnbranchedTests();
    assert.equal(node.branchName, 'a/b/');
    assert.equal(node.children.length, 2);
    assert.equal(node.children[0].branchName, 'c/');
    assert.equal(node.children[0].children.length, 2);
    assert.equal(node.children[0].children[0].branchName, 'd');
    assert.equal(node.children[0].children[1].branchName, 'e');
    assert.equal(node.children[1].branchName, 'f');
  });
});

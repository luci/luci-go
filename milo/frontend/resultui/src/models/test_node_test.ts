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
    node.addTestId('a/b/c/d');
    node.addTestId('a/b/c/e');

    assert.equal(node.path, 'a/b/c/');
    assert.equal(node.name, 'a/b/c/');
    assert.equal(node.children.length, 2);

    assert.equal(node.children[0].path, 'a/b/c/d');
    assert.equal(node.children[0].name, 'd');
    assert.equal(node.children[0].children.length, 0);

    assert.equal(node.children[1].path, 'a/b/c/e');
    assert.equal(node.children[1].name, 'e');
    assert.equal(node.children[1].children.length, 0);
  });

  it('should un-elide test id segments after a new branch is added', async () => {
    const node = TestNode.newRoot();
    node.addTestId('a/b/c/d');
    node.addTestId('a/b/c/e');
    assert.equal(node.name, 'a/b/c/');
    assert.equal(node.children.length, 2);

    node.addTestId('a/b/f');
    assert.equal(node.name, 'a/b/');
    assert.equal(node.children.length, 2);

    assert.equal(node.children[0].name, 'c/');
    assert.equal(node.children[0].children.length, 2);
    assert.equal(node.children[0].children[0].name, 'd');
    assert.equal(node.children[0].children[1].name, 'e');
    assert.equal(node.children[1].name, 'f');
  });

  it('should not elide test ids when one is the prefix of another', async () => {
    const node = TestNode.newRoot();
    // 'a/b/c/d/' should not be treated as the parent of 'a/b/c/d/e'.
    // All tests should be treated as leaves.
    node.addTestId('a/b/c/d/');
    node.addTestId('a/b/c/d/e');
    assert.equal(node.name, 'a/b/c/d/');
    assert.equal(node.children.length, 2);
    assert.equal(node.children[0].name, '');
    assert.equal(node.children[1].name, 'e');
  });
});

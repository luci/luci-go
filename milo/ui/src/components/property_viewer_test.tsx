// Copyright 2023 The LUCI Authors.
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

import { cleanup, render } from '@testing-library/react';
import { expect } from 'chai';
import CodeMirror from 'codemirror';
import { destroy } from 'mobx-state-tree';
import * as sinon from 'sinon';

import { PropertyViewerConfig, PropertyViewerConfigInstance } from '../store/user_config';
import { PropertyViewer } from './property_viewer';

const TEST_PROPERTIES = {
  key1: {
    child1: 'value1',
  },
  key2: {
    child2: 'value2',
  },
};

const TEST_PROPERTIES_LINES = JSON.stringify(TEST_PROPERTIES, undefined, 2).split('\n');

describe('PropertyViewer', () => {
  let timer: sinon.SinonFakeTimers;
  let store: PropertyViewerConfigInstance;

  beforeEach(() => {
    timer = sinon.useFakeTimers();
    store = PropertyViewerConfig.create({});
  });

  afterEach(() => {
    cleanup();
    destroy(store);
    timer.restore();
  });

  it('e2e', async () => {
    store.setFolded('  "key1": {', true);
    store.setFolded('  "key3": {', true);

    await timer.tickAsync('01:00:00');
    const beforeRender = timer.now;

    // Renders properties.
    await timer.tickAsync('01:00:00');

    let editor: CodeMirror.Editor;
    render(
      <PropertyViewer
        properties={TEST_PROPERTIES}
        config={store}
        onInit={(e) => {
          editor = e;
        }}
      />
    );
    await timer.runAllAsync();

    expect(store.isFolded('  "key1": {')).to.be.true;
    expect(store.isFolded('  "key2": {')).to.be.false;
    expect(store.isFolded('  "key3": {')).to.be.true;

    store.deleteStaleKeys(new Date(beforeRender));
    // Rendered key's fold state should be refreshed.
    expect(store.isFolded('  "key1": {')).to.be.true;
    // Un-rendered key's fold state should not be refreshed.
    // So stale keys won't take up memory/disk space.
    expect(store.isFolded('  "key2": {')).to.be.false;
    expect(store.isFolded('  "key3": {')).to.be.false;

    await timer.runAllAsync();

    // Unfold key1.
    editor!.foldCode(TEST_PROPERTIES_LINES.indexOf('  "key1": {'), undefined, 'unfold');
    await timer.runAllAsync();

    expect(store.isFolded('  "key1": {')).to.be.false;
    expect(store.isFolded('  "key2": {')).to.be.false;
    expect(store.isFolded('  "key3": {')).to.be.false;

    // Fold key2.
    editor!.foldCode(TEST_PROPERTIES_LINES.indexOf('  "key2": {'), undefined, 'fold');
    await timer.runAllAsync();

    expect(store.isFolded('  "key1": {')).to.be.false;
    expect(store.isFolded('  "key2": {')).to.be.true;
    expect(store.isFolded('  "key3": {')).to.be.false;
  });
});

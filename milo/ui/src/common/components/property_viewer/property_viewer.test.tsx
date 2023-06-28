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
import CodeMirror from 'codemirror';
import { destroy } from 'mobx-state-tree';

import {
  PropertyViewerConfig,
  PropertyViewerConfigInstance,
} from '@/common/store/user_config/build_config';

import { PropertyViewer } from './property_viewer';

const TEST_PROPERTIES = {
  key1: {
    child1: 'value1',
  },
  key2: {
    child2: 'value2',
  },
};

const TEST_PROPERTIES_LINES = JSON.stringify(
  TEST_PROPERTIES,
  undefined,
  2
).split('\n');

// jsdom doesn't allow us to unit test a codemirror editor.
// eslint-disable-next-line jest/no-disabled-tests
describe.skip('PropertyViewer', () => {
  let store: PropertyViewerConfigInstance;

  beforeEach(() => {
    jest.useFakeTimers();
    store = PropertyViewerConfig.create({});
  });

  afterEach(() => {
    cleanup();
    destroy(store);
    jest.useRealTimers();
  });

  test('e2e', async () => {
    store.setFolded('  "key1": {', true);
    store.setFolded('  "key3": {', true);

    await jest.advanceTimersByTimeAsync(1000 * 60 * 60);
    const beforeRender = jest.now();

    // Renders properties.
    await jest.advanceTimersByTimeAsync(1000 * 60 * 60);

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
    await jest.runAllTimersAsync();

    expect(store.isFolded('  "key1": {')).toBeTruthy();
    expect(store.isFolded('  "key2": {')).toBeFalsy();
    expect(store.isFolded('  "key3": {')).toBeTruthy();

    store.deleteStaleKeys(new Date(beforeRender));
    // Rendered key's fold state should be refreshed.
    expect(store.isFolded('  "key1": {')).toBeTruthy();
    // Un-rendered key's fold state should not be refreshed.
    // So stale keys won't take up memory/disk space.
    expect(store.isFolded('  "key2": {')).toBeFalsy();
    expect(store.isFolded('  "key3": {')).toBeFalsy();

    await jest.runAllTimersAsync();

    // Unfold key1.
    editor!.foldCode(
      TEST_PROPERTIES_LINES.indexOf('  "key1": {'),
      undefined,
      'unfold'
    );
    await jest.runAllTimersAsync();

    expect(store.isFolded('  "key1": {')).toBeFalsy();
    expect(store.isFolded('  "key2": {')).toBeFalsy();
    expect(store.isFolded('  "key3": {')).toBeFalsy();

    // Fold key2.
    editor!.foldCode(
      TEST_PROPERTIES_LINES.indexOf('  "key2": {'),
      undefined,
      'fold'
    );
    await jest.runAllTimersAsync();

    expect(store.isFolded('  "key1": {')).toBeFalsy();
    expect(store.isFolded('  "key2": {')).toBeTruthy();
    expect(store.isFolded('  "key3": {')).toBeFalsy();
  });
});

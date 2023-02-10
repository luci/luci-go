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

import { FileDescriptorSet, Descriptors } from './prpc';
import { TestDescriptor } from './testdata/descriptor';

const descs = new Descriptors(
  TestDescriptor as FileDescriptorSet,
  ['rpcexplorer.S1', 'rpcexplorer.S2'],
);

/* eslint-disable @typescript-eslint/no-non-null-assertion */

describe('Descriptors', () => {
  it('Discovers all services and methods', () => {
    expect(descs.service('rpcexplorer.S1')).toBeDefined();
    expect(descs.service('rpcexplorer.S2')).toBeDefined();

    const s1 = descs.service('rpcexplorer.S1')!;
    expect(s1.method('Method')).toBeDefined();

    const s2 = descs.service('rpcexplorer.S2')!;
    expect(s2.methods).toHaveLength(0);
  });

  it('Discovers service comment', () => {
    expect(descs.service('rpcexplorer.S1')!.help).toEqual('Does something.');
    expect(descs.service('rpcexplorer.S2')!.help).toEqual('');
  });

  it('Discovers method comment', () => {
    expect(
      descs.service('rpcexplorer.S1')!.method('Method')!.help,
    ).toEqual('Does blah.');
  });

  it('Discovers service doc', () => {
    expect(descs.service('rpcexplorer.S1')!.doc).toEqual(
        'S1 service does something.' +
      '\n\n' +
      'And one more line.',
    );
  });

  it('Discovers method doc', () => {
    expect(descs.service('rpcexplorer.S1')!.method('Method')!.doc).toEqual(
        'Method does blah.' +
      '\n\n' +
      'And something extra.',
    );
  });
});

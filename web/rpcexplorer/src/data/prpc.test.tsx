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

import { FileDescriptorSet, Descriptors, JSONType } from './prpc';
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

  it('Fields', () => {
    const msg = descs.message('rpcexplorer.M');
    expect(msg!.fieldByJsonName('i')!.jsonName).toEqual('i');
    expect(msg!.fieldByJsonName('m')!.repeated).toEqual(false);
    expect(msg!.fieldByJsonName('mr')!.repeated).toEqual(true);
  });

  it('Fields types', () => {
    let m = descs.message('rpcexplorer.M');
    expect(m!.fieldByJsonName('i')!.type).toEqual('int32');
    expect(m!.fieldByJsonName('s')!.type).toEqual('string');
    expect(m!.fieldByJsonName('ri')!.type).toEqual('repeated int32');
    expect(m!.fieldByJsonName('e')!.type).toEqual('rpcexplorer.E');
    expect(m!.fieldByJsonName('m')!.type).toEqual('rpcexplorer.M2');

    m = descs.message('rpcexplorer.MapContainer');
    expect(m!.fieldByJsonName('im')!.type).toEqual('map<int32, rpcexplorer.M>');
  });

  it('Fields JSON types', () => {
    let m = descs.message('rpcexplorer.M');
    expect(m!.fieldByJsonName('i')!.jsonType).toEqual(JSONType.Scalar);
    expect(m!.fieldByJsonName('s')!.jsonType).toEqual(JSONType.String);
    expect(m!.fieldByJsonName('ri')!.jsonType).toEqual(JSONType.List);
    expect(m!.fieldByJsonName('ri')!.jsonElementType).toEqual(JSONType.Scalar);
    expect(m!.fieldByJsonName('e')!.jsonType).toEqual(JSONType.String);
    expect(m!.fieldByJsonName('m')!.jsonType).toEqual(JSONType.Object);

    m = descs.message('rpcexplorer.MapContainer');
    expect(m!.fieldByJsonName('im')!.jsonType).toEqual(JSONType.Object);
  });

  it('Fields doc', () => {
    const m = descs.message('rpcexplorer.M');
    expect(m!.fieldByJsonName('i')!.doc).toEqual('i is integer');
    expect(m!.fieldByJsonName('mr')!.doc).toEqual(
        'mr is repeated message\nsecond line.',
    );
  });

  it('Well-known types in fields', () => {
    const m = descs.message('rpcexplorer.Autocomplete');
    expect(m!.fieldByJsonName('anyMsg')!.jsonType).toEqual(JSONType.Object);
    expect(m!.fieldByJsonName('durMsg')!.jsonType).toEqual(JSONType.String);
    expect(m!.fieldByJsonName('tsMsg')!.jsonType).toEqual(JSONType.String);
    expect(m!.fieldByJsonName('fmMsg')!.jsonType).toEqual(JSONType.String);
    expect(m!.fieldByJsonName('structMsg')!.jsonType).toEqual(JSONType.Object);
    expect(m!.fieldByJsonName('strMsg')!.jsonType).toEqual(JSONType.String);
    expect(m!.fieldByJsonName('intMsg')!.jsonType).toEqual(JSONType.Scalar);
  });

  it('Enums', () => {
    const e = descs.message('rpcexplorer.M')!.fieldByJsonName('e')!.enum!;
    expect(e.values[0].name).toEqual('V0');
    expect(e.values[0].doc).toEqual('V0 comment.');
  });
});

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

import { getBaseNodeId } from './get_base_node_id';

describe('getBaseNodeId', () => {
  it('should return undefined for undefined input', () => {
    expect(getBaseNodeId(undefined)).toBeUndefined();
  });

  it('should extract base stage ID from full node ID (default includes prefix)', () => {
    expect(getBaseNodeId('Lworkplan123:Nstage456')).toEqual('Nstage456');
  });

  it('should extract base stage ID from full node ID with prefix stripped', () => {
    expect(
      getBaseNodeId('Lworkplan123:Nstage456', { includePrefix: false }),
    ).toEqual('stage456');
  });

  it('should extract base stage ID from local node ID with attempt suffix (default includes prefix)', () => {
    expect(getBaseNodeId('S$init:A1')).toEqual('S$init');
    expect(getBaseNodeId('Nstage456:A2')).toEqual('Nstage456');
  });

  it('should extract base stage ID with prefix stripped', () => {
    expect(getBaseNodeId('S$init:A1', { includePrefix: false })).toEqual(
      '$init',
    );
    expect(getBaseNodeId('Nstage456:A2', { includePrefix: false })).toEqual(
      'stage456',
    );
  });

  it('should extract base check ID from local node ID with result suffix (default includes prefix)', () => {
    expect(getBaseNodeId('Ccheck123:R1')).toEqual('Ccheck123');
  });

  it('should extract base check ID with prefix stripped', () => {
    expect(getBaseNodeId('Ccheck123:R1', { includePrefix: false })).toEqual(
      'check123',
    );
  });

  it('should return the original ID if it has no type prefix/suffix', () => {
    expect(getBaseNodeId('stage_without_prefix')).toEqual(
      'stage_without_prefix',
    );
    expect(
      getBaseNodeId('stage_without_prefix', { includePrefix: false }),
    ).toEqual('stage_without_prefix');
  });
});

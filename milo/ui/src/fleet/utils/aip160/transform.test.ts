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

import { remapFilterDimensions } from './transform';

describe('remapFilterDimensions', () => {
  const testMapping = {
    model: 'labels.model',
    models: 'labels.model',
    pool: 'labels.pool',
    pools: 'labels.pool',
    device_id: 'id',
  };

  it('handles empty and whitespace input', () => {
    expect(remapFilterDimensions('', testMapping)).toBe('');
    expect(remapFilterDimensions('   ', testMapping)).toBe('');
    expect(remapFilterDimensions(undefined, testMapping)).toBe('');
    expect(remapFilterDimensions(null, testMapping)).toBe('');
  });

  it('remaps simple top-level dimension keys', () => {
    expect(remapFilterDimensions('model = "brya"', testMapping)).toBe(
      'labels.model = "brya"',
    );
    expect(remapFilterDimensions('pool = "DUT_POOL_QUOTA"', testMapping)).toBe(
      'labels.pool = "DUT_POOL_QUOTA"',
    );
    expect(remapFilterDimensions('device_id = "device-123"', testMapping)).toBe(
      'id = "device-123"',
    );
  });

  it('preserves already qualified fields', () => {
    expect(remapFilterDimensions('labels.model = "brya"', testMapping)).toBe(
      'labels.model = "brya"',
    );
    expect(remapFilterDimensions('labels.pool = "quota"', testMapping)).toBe(
      'labels.pool = "quota"',
    );
    expect(
      remapFilterDimensions('labels."dut_state" = "ready"', testMapping),
    ).toBe('labels."dut_state" = "ready"');
  });

  it('does not touch dimension values that match mapped field names', () => {
    // If a tag value happens to contain "model", it must not be altered
    expect(remapFilterDimensions('labels.tag = "model"', testMapping)).toBe(
      'labels.tag = "model"',
    );
  });

  it('remaps compound AND conjunctions', () => {
    const input = 'model = "brya" AND pool = "DUT_POOL_QUOTA"';
    expect(remapFilterDimensions(input, testMapping)).toBe(
      'labels.model = "brya" AND labels.pool = "DUT_POOL_QUOTA"',
    );
  });

  it('remaps compound OR disjunctions within parentheses', () => {
    const input =
      '(model = "brya" OR model = "corsola") AND pool = "DUT_POOL_QUOTA"';
    expect(remapFilterDimensions(input, testMapping)).toBe(
      '(labels.model = "brya" OR labels.model = "corsola") AND labels.pool = "DUT_POOL_QUOTA"',
    );
  });

  it('handles negated terms', () => {
    expect(remapFilterDimensions('-model = "brya"', testMapping)).toBe(
      '-labels.model = "brya"',
    );
  });

  it('falls back gracefully to original string on syntax error', () => {
    const invalidSyntax = 'model === "brya" ??? invalid';
    expect(remapFilterDimensions(invalidSyntax, testMapping)).toBe(
      invalidSyntax,
    );
  });
});

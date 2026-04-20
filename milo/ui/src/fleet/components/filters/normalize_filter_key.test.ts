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

import { normalizeFilterKey } from './normalize_filter_key';

describe('normalizeFilterKey', () => {
  it('should remove labels. prefix', () => {
    expect(normalizeFilterKey('labels.dut_state')).toEqual('dut_state');
  });

  it('should remove quotes', () => {
    expect(normalizeFilterKey('"dut_state"')).toEqual('dut_state');
  });

  it('should remove both', () => {
    expect(normalizeFilterKey('labels."dut_state"')).toEqual('dut_state');
  });

  it('should leave other keys unchanged', () => {
    expect(normalizeFilterKey('dut_state')).toEqual('dut_state');
  });
});

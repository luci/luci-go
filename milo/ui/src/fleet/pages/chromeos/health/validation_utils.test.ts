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

import { MAX_QUOTA, validateQuota } from './validation_utils';

describe('validateQuota', () => {
  it('accepts valid positive integers', () => {
    expect(validateQuota('1')).toBe('');
    expect(validateQuota('10')).toBe('');
    expect(validateQuota('100000')).toBe('');
    expect(validateQuota(String(MAX_QUOTA))).toBe('');
  });

  it('rejects empty or whitespace-only inputs', () => {
    expect(validateQuota('')).toBe(
      'Expected quota must be a positive whole integer greater than 0.',
    );
    expect(validateQuota('   ')).toBe(
      'Expected quota must be a positive whole integer greater than 0.',
    );
  });

  it('rejects zero and negative numbers', () => {
    expect(validateQuota('0')).toBe(
      'Expected quota must be a positive whole integer greater than 0.',
    );
    expect(validateQuota('-5')).toBe(
      'Expected quota must be a positive whole integer greater than 0.',
    );
  });

  it('rejects non-integer values and strings', () => {
    expect(validateQuota('3.14')).toBe(
      'Expected quota must be a positive whole integer greater than 0.',
    );
    expect(validateQuota('abc')).toBe(
      'Expected quota must be a positive whole integer greater than 0.',
    );
  });

  it('rejects numbers exceeding MAX_QUOTA', () => {
    expect(validateQuota(String(MAX_QUOTA + 1))).toBe(
      `Expected quota cannot exceed ${MAX_QUOTA.toLocaleString()}.`,
    );
  });
});

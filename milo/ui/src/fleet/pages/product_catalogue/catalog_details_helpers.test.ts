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

import {
  copyTextToClipboard,
  formatProductCategory,
  formatUnitCost,
  resolveProductType,
} from './catalog_details_helpers';

describe('catalog_details_helpers', () => {
  it('resolves GCE product type for both GC- and GCE- prefixes', () => {
    expect(resolveProductType(undefined, 'GC-000')).toBe('gce');
    expect(resolveProductType(undefined, 'GCE-001')).toBe('gce');
    expect(formatProductCategory(undefined, 'GC-000')).toBe(
      'GCE Virtual Machine',
    );
    expect(formatProductCategory(undefined, 'GCE-001')).toBe(
      'GCE Virtual Machine',
    );
  });

  it('formats numeric unitCost with or without a leading dollar sign and returns em-dash for zero', () => {
    expect(formatUnitCost(undefined)).toBe('—');
    expect(formatUnitCost('')).toBe('—');
    expect(formatUnitCost('0')).toBe('—');
    expect(formatUnitCost('$0')).toBe('—');
    expect(formatUnitCost('$0.00')).toBe('—');
    expect(formatUnitCost('26645')).toBe('$26,645');
    expect(formatUnitCost('$26645')).toBe('$26,645');
    expect(formatUnitCost('$26,645')).toBe('$26,645');
    expect(formatUnitCost('TBD')).toBe('TBD');
  });

  it('handles copyTextToClipboard safely when navigator.clipboard is unavailable', async () => {
    const originalClipboard = navigator.clipboard;
    Object.defineProperty(navigator, 'clipboard', {
      value: undefined,
      configurable: true,
    });
    document.execCommand = jest.fn().mockReturnValue(true);
    try {
      await expect(copyTextToClipboard('sample prompt')).resolves.toBe(true);
      expect(document.execCommand).toHaveBeenCalledWith('copy');
    } finally {
      Object.defineProperty(navigator, 'clipboard', {
        value: originalClipboard,
        configurable: true,
      });
    }
  });
});

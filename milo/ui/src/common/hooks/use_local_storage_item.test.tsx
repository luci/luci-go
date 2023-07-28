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

import { act, renderHook } from '@testing-library/react';

import { useLocalStorageItem } from './use_local_storage_item';

describe('useLocalStorageItem', () => {
  const memoryStorage = new Map<string, string>();

  beforeEach(() => {
    jest
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation((key: string, value: string) => {
        memoryStorage.set(key, value);
      });
    jest
      .spyOn(Storage.prototype, 'getItem')
      .mockImplementation((key: string) => {
        return memoryStorage.get(key) || null;
      });
  });

  afterEach(() => {
    jest.restoreAllMocks();
    memoryStorage.clear();
  });

  it('should set value of item', () => {
    const { result } = renderHook(() => useLocalStorageItem('key'));
    expect(result.current[0]).toBe('');

    act(() => result.current[1]('new_value'));

    expect(memoryStorage.get('key')).toBe('new_value');
  });

  it('should use default value', () => {
    const { result } = renderHook(() => useLocalStorageItem('key', 'default'));
    expect(result.current[0]).toBe('default');
    expect(memoryStorage.get('key')).toBe('default');
  });
});

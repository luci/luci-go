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
  safeLocalStorageGet,
  safeLocalStorageRemove,
  safeLocalStorageSet,
} from './storage';

describe('storage utils', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('safeLocalStorageGet', () => {
    it('returns stored value when key exists', () => {
      localStorage.setItem('test-key', 'test-value');
      expect(safeLocalStorageGet('test-key')).toBe('test-value');
    });

    it('returns null when key does not exist', () => {
      expect(safeLocalStorageGet('non-existent')).toBeNull();
    });

    it('handles localStorage errors gracefully and calls onError', () => {
      const error = new Error('Storage access denied');
      jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
        throw error;
      });
      const onError = jest.fn();

      const result = safeLocalStorageGet('any-key', onError);

      expect(result).toBeNull();
      expect(onError).toHaveBeenCalledTimes(1);
      expect(onError).toHaveBeenCalledWith(error);
    });

    it('handles localStorage errors gracefully without onError callback', () => {
      jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
        throw new Error('Storage access denied');
      });

      expect(safeLocalStorageGet('any-key')).toBeNull();
    });
  });

  describe('safeLocalStorageSet', () => {
    it('stores value and returns true', () => {
      const result = safeLocalStorageSet('test-key', 'test-value');

      expect(result).toBe(true);
      expect(localStorage.getItem('test-key')).toBe('test-value');
    });

    it('handles localStorage quota/security errors and calls onError', () => {
      const error = new Error('QuotaExceededError');
      jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw error;
      });
      const onError = jest.fn();

      const result = safeLocalStorageSet('test-key', 'test-value', onError);

      expect(result).toBe(false);
      expect(onError).toHaveBeenCalledTimes(1);
      expect(onError).toHaveBeenCalledWith(error);
    });

    it('handles errors gracefully without onError callback', () => {
      jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw new Error('QuotaExceededError');
      });

      expect(safeLocalStorageSet('test-key', 'test-value')).toBe(false);
    });
  });

  describe('safeLocalStorageRemove', () => {
    it('removes value and returns true', () => {
      localStorage.setItem('test-key', 'test-value');

      const result = safeLocalStorageRemove('test-key');

      expect(result).toBe(true);
      expect(localStorage.getItem('test-key')).toBeNull();
    });

    it('handles localStorage errors and calls onError', () => {
      const error = new Error('SecurityError');
      jest.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
        throw error;
      });
      const onError = jest.fn();

      const result = safeLocalStorageRemove('test-key', onError);

      expect(result).toBe(false);
      expect(onError).toHaveBeenCalledTimes(1);
      expect(onError).toHaveBeenCalledWith(error);
    });

    it('handles errors gracefully without onError callback', () => {
      jest.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
        throw new Error('SecurityError');
      });

      expect(safeLocalStorageRemove('test-key')).toBe(false);
    });
  });
});

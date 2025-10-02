// Copyright 2025 The LUCI Authors.
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

import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { RootInvocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';

import {
  AnyInvocation,
  isRootInvocation,
  isLegacyInvocation,
} from './invocation_utils';

// Mocks based on the fields the guards check
const mockRootInvocation = {
  name: 'rootInvocations/root-123',
  rootInvocationId: 'root-123',
  realm: 'test:realm',
} as RootInvocation;

const mockLegacyInvocation = {
  name: 'invocations/inv-123',
  isExportRoot: true,
  realm: 'test:realm',
} as Invocation;

describe('invocation_utils', () => {
  describe('isRootInvocation', () => {
    it('should return true for a RootInvocation', () => {
      expect(isRootInvocation(mockRootInvocation as AnyInvocation)).toBe(true);
    });

    it('should return false for a legacy Invocation', () => {
      expect(isRootInvocation(mockLegacyInvocation as AnyInvocation)).toBe(
        false,
      );
    });

    it('should return false for null', () => {
      expect(isRootInvocation(null)).toBe(false);
    });

    it('should return false for undefined', () => {
      expect(isRootInvocation(undefined)).toBe(false);
    });
  });

  describe('isLegacyInvocation', () => {
    it('should return true for a legacy Invocation', () => {
      expect(isLegacyInvocation(mockLegacyInvocation as AnyInvocation)).toBe(
        true,
      );
    });

    it('should return false for a RootInvocation', () => {
      expect(isLegacyInvocation(mockRootInvocation as AnyInvocation)).toBe(
        false,
      );
    });

    it('should return false for null', () => {
      expect(isLegacyInvocation(null)).toBe(false);
    });

    it('should return false for undefined', () => {
      expect(isLegacyInvocation(undefined)).toBe(false);
    });
  });
});

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

import { OutputTestVerdict } from '@/common/types/verdict';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import {
  escapeShellArg,
  getAtestCommand,
  isVirtualTarget,
} from './rerun_utils';

describe('rerun_button utilities', () => {
  describe('escapeShellArg', () => {
    it('handles empty or undefined string', () => {
      expect(escapeShellArg('')).toBe("''");
      expect(escapeShellArg(undefined)).toBe("''");
    });

    it('wraps normal string in single quotes', () => {
      expect(escapeShellArg('hello')).toBe("'hello'");
      expect(escapeShellArg('hello world')).toBe("'hello world'");
    });

    it('escapes internal single quotes safely for POSIX shell', () => {
      expect(escapeShellArg("foo'bar")).toBe("'foo'\\''bar'");
      expect(escapeShellArg("'")).toBe("''\\'''");
    });

    it('safely neutralizes shell metacharacters within single quotes', () => {
      expect(escapeShellArg('foo"bar')).toBe("'foo\"bar'");
      expect(escapeShellArg('foo$bar')).toBe("'foo$bar'");
      expect(escapeShellArg('foo`bar`')).toBe("'foo`bar`'");
      expect(escapeShellArg('foo; rm -rf /')).toBe("'foo; rm -rf /'");
    });
  });

  describe('isVirtualTarget', () => {
    it('returns true for valid virtual targets', () => {
      expect(isVirtualTarget('cf_x86_64_phone-userdebug')).toBe(true);
      expect(
        isVirtualTarget('aosp_cf_arm64_phone-trunk_staging-userdebug'),
      ).toBe(true);
      expect(isVirtualTarget('cuttlestone_x86-userdebug')).toBe(true);
      expect(isVirtualTarget('gce_x86_64-userdebug')).toBe(true);
    });

    it('returns false for non-virtual targets', () => {
      expect(isVirtualTarget('oriole-userdebug')).toBe(false);
      expect(isVirtualTarget('husky-userdebug')).toBe(false);
      expect(isVirtualTarget(null)).toBe(false);
      expect(isVirtualTarget(undefined)).toBe(false);
      expect(isVirtualTarget('')).toBe(false);
    });

    it('rejects targets containing shell injection characters even if prefixed with virtual prefix', () => {
      expect(isVirtualTarget('cf_x86"; touch /tmp/pwn #')).toBe(false);
      expect(isVirtualTarget('cf_x86$(touch /tmp/pwn)')).toBe(false);
      expect(isVirtualTarget('cf_x86`touch /tmp/pwn`')).toBe(false);
      expect(isVirtualTarget('cf_x86 && rm -rf /')).toBe(false);
      expect(isVirtualTarget('cf_x86\nwhoami')).toBe(false);
    });
  });

  describe('getAtestCommand', () => {
    const mockVerdict = TestVariant.fromPartial({
      testId: 'test/id/some.Test',
      testIdStructured: {
        moduleName: 'CtsExampleTestCases',
        coarseName: 'android.example',
        fineName: 'ExampleTest',
        caseName: 'testFoo',
      },
      variant: {
        def: {
          module_abi: 'arm64-v8a',
        },
      },
    }) as OutputTestVerdict;

    it('returns standard atest command with properly quoted identifier', () => {
      const cmd = getAtestCommand(mockVerdict);
      expect(cmd).toBe(
        "atest 'CtsExampleTestCases:android.example.ExampleTest#testFoo' -- --abi 'arm64-v8a'",
      );
    });

    it('supports moduleOnly option', () => {
      const cmd = getAtestCommand(mockVerdict, { moduleOnly: true });
      expect(cmd).toBe("atest 'CtsExampleTestCases' -- --abi 'arm64-v8a'");
    });

    it('supports omitAtest and omitExtraArgs options', () => {
      const cmd = getAtestCommand(mockVerdict, {
        omitAtest: true,
        omitExtraArgs: true,
      });
      expect(cmd).toBe(
        "'CtsExampleTestCases:android.example.ExampleTest#testFoo'",
      );
    });

    it('generates safe acloud rerun command when valid acloudBuild is provided', () => {
      const cmd = getAtestCommand(mockVerdict, {
        acloudBuild: {
          branch: 'git_main',
          buildTarget: 'cf_x86_64_phone-userdebug',
          buildId: '123456',
        },
      });
      expect(cmd).toBe(
        "atest 'CtsExampleTestCases:android.example.ExampleTest#testFoo' " +
          "--acloud-create '--branch git_main --build-target cf_x86_64_phone-userdebug --build-id 123456' " +
          "-- --abi 'arm64-v8a'",
      );
    });

    it('returns null when acloudBuild branch contains injection payload (b/550465350)', () => {
      const cmdDquote = getAtestCommand(mockVerdict, {
        acloudBuild: {
          branch: 'git_main"; touch /tmp/pwn #',
          buildTarget: 'cf_x86_64_phone-userdebug',
          buildId: '123456',
        },
      });
      expect(cmdDquote).toBeNull();

      const cmdSubst = getAtestCommand(mockVerdict, {
        acloudBuild: {
          branch: 'git_main$(touch /tmp/pwn)',
          buildTarget: 'cf_x86_64_phone-userdebug',
          buildId: '123456',
        },
      });
      expect(cmdSubst).toBeNull();
    });

    it('returns null when acloudBuild buildTarget contains injection payload (b/550465350)', () => {
      const cmd = getAtestCommand(mockVerdict, {
        acloudBuild: {
          branch: 'git_main',
          buildTarget: 'cf_x86"; touch /tmp/pwn #',
          buildId: '123456',
        },
      });
      expect(cmd).toBeNull();
    });

    it('returns null when acloudBuild buildId contains injection payload (b/550465350)', () => {
      const cmd = getAtestCommand(mockVerdict, {
        acloudBuild: {
          branch: 'git_main',
          buildTarget: 'cf_x86_64_phone-userdebug',
          buildId: '123456"; rm -rf / #',
        },
      });
      expect(cmd).toBeNull();
    });

    it('returns null when acloudBuild target is not a virtual target', () => {
      const cmd = getAtestCommand(mockVerdict, {
        acloudBuild: {
          branch: 'git_main',
          buildTarget: 'oriole-userdebug',
          buildId: '123456',
        },
      });
      expect(cmd).toBeNull();
    });

    it('returns null when acloudBuild parameters are missing', () => {
      const cmd = getAtestCommand(mockVerdict, {
        acloudBuild: {
          branch: '',
          buildTarget: 'cf_x86_64_phone-userdebug',
          buildId: '123456',
        },
      });
      expect(cmd).toBeNull();
    });

    it('ignores invalid module_abi containing shell metacharacters', () => {
      const verdictMaliciousAbi = TestVariant.fromPartial({
        ...mockVerdict,
        variant: {
          def: {
            module_abi: 'arm64; rm -rf /',
          },
        },
      }) as OutputTestVerdict;

      const cmd = getAtestCommand(verdictMaliciousAbi);
      expect(cmd).toBe(
        "atest 'CtsExampleTestCases:android.example.ExampleTest#testFoo'",
      );
    });
  });
});

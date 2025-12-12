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

import {
  AggregationLevel,
  CompoundValue,
  hashString,
  invocationHash,
  InvocationID,
  quote,
  SimpleValue,
  TestID,
  testIdentifierHash,
} from './index';

describe('HashString', () => {
  it('HashString', () => {
    const result = hashString('The quick brown fox jumps over the lazy dog');
    expect(result).toBe('6c1b07bc7bbc4be347939ac4a93c437a');
  });
});

describe('TestHashing', () => {
  describe('TestInvocationHash', () => {
    const invocationBase: InvocationID = {
      branch: 'git_master',
      target: 'sailfish-userdebug',
      buildProvider: '',
      scheduler: 'scheduler',
      testName: 'example/test',
      properties: {
        test_property_1: 'test_property_value_1',
        test_property_2: 'test_property_value_2',
      },
    };

    it('Base', () => {
      expect(invocationHash(invocationBase)).toBe(
        '141a2163b9794e78eee68689470db72a',
      );
    });
    it('Another build', () => {
      const invocation = { ...invocationBase, branch: 'another_branch' };
      expect(invocationHash(invocation)).toBe(
        '76fa52a8379ea9a8531213ffc9042130',
      );
    });
  });

  describe('TestIdentifierHash', () => {
    let invocation: InvocationID;
    let testID: TestID;

    beforeEach(() => {
      invocation = {
        branch: 'git_master',
        target: 'sailfish-userdebug',
        buildProvider: '',
        scheduler: 'scheduler',
        testName: 'example/test',
        properties: {
          test_property_1: 'test_property_value_1',
          test_property_2: 'test_property_value_2',
        },
      };

      testID = {
        module: 'module',
        moduleParameters: {
          module_param_1: 'module_param_value_1',
          module_param_2: 'module_param_value_2',
        },
        testClass: 'com.example.ExampleTest',
        method: 'testMethod',
        aggregationLevel: AggregationLevel.UNSPECIFIED,
      };
    });

    // Expected hashes
    const granularTestResultHash = '3e06356e0075e44e824237638ddd31f8';
    const classLevelTestResultHash = 'c3e3ebbc5abfec7962f195656374fd63';
    const packageLevelTestResultHash = '3167d6a1680f35f6c6fa517ed24a1b0d';
    const moduleTestResultHash = 'a1f1bfc4f31b0a80748d88b43b89eca3';
    const invocationTestResultHash = '943e6b93085e47802597926b8495411d';

    const emptyGranularTestResultHash = '6299002389f4a29377d985ca1628baf7';
    const emptyClassTestResultHash = '0682cfefac93ffe7f0ec449a7287380f';
    const emptyPackageTestResultHash = 'b7b9159da62f575a3dbb713a5b615ae1';
    const emptyModuleTestResultHash = 'ea5b77b1a10abb45f53cd84d235910a3';
    const emptyModulePackageTestResultHash = '030fc35a2e36b9b40c95058d28774661';

    it('Full test ID', () => {
      expect(testIdentifierHash(invocation, testID)).toBe(
        granularTestResultHash,
      );
    });

    it('Empty method test ID', () => {
      const tid = { ...testID, method: '' };
      expect(testIdentifierHash(invocation, tid)).toBe(
        emptyGranularTestResultHash,
      );
    });

    describe('Method-level aggregates', () => {
      let tid: TestID;
      beforeEach(() => {
        tid = { ...testID, aggregationLevel: AggregationLevel.METHOD };
      });
      it('Base', () => {
        expect(testIdentifierHash(invocation, tid)).toBe(
          granularTestResultHash,
        );
      });
      it('Without method', () => {
        tid = { ...tid, method: '' };
        expect(testIdentifierHash(invocation, tid)).toBe(
          emptyGranularTestResultHash,
        );
      });
    });

    describe('Class-level aggregates', () => {
      let tid: TestID;
      beforeEach(() => {
        tid = {
          ...testID,
          method: '',
          aggregationLevel: AggregationLevel.CLASS,
        };
      });
      it('Base', () => {
        expect(testIdentifierHash(invocation, tid)).toBe(
          classLevelTestResultHash,
        );
      });
      it('No class', () => {
        tid = { ...tid, testClass: '' };
        expect(testIdentifierHash(invocation, tid)).toBe(
          emptyClassTestResultHash,
        );
      });
    });

    describe('Package-level aggregates', () => {
      let tid: TestID;
      beforeEach(() => {
        tid = {
          ...testID,
          method: '',
          aggregationLevel: AggregationLevel.PACKAGE,
        };
      });
      it('Base', () => {
        tid = { ...tid, testClass: 'com.example.*' };
        expect(testIdentifierHash(invocation, tid)).toBe(
          packageLevelTestResultHash,
        );
      });
      it('No package', () => {
        tid = { ...tid, testClass: '' };
        expect(testIdentifierHash(invocation, tid)).toBe(
          emptyPackageTestResultHash,
        );
      });
      it('No module', () => {
        // No module + No module params
        tid = {
          ...tid,
          testClass: '',
          module: '',
          moduleParameters: undefined,
        };
        // Note: we can use undefined or {} for moduleParameters as handled by our code logic
        expect(testIdentifierHash(invocation, tid)).toBe(
          emptyModulePackageTestResultHash,
        );
      });
    });

    describe('Module-level aggregates', () => {
      let tid: TestID;
      beforeEach(() => {
        tid = {
          ...testID,
          method: '',
          testClass: '',
          aggregationLevel: AggregationLevel.MODULE,
        };
      });
      it('Base', () => {
        expect(testIdentifierHash(invocation, tid)).toBe(moduleTestResultHash);
      });
      it('No Module', () => {
        tid = {
          ...tid,
          module: '',
          moduleParameters: undefined,
        };
        expect(testIdentifierHash(invocation, tid)).toBe(
          emptyModuleTestResultHash,
        );
      });
    });

    describe('Invocation-level aggregates', () => {
      it('Base', () => {
        // Ensure other fields are defaults (empty) if constructing partial
        const tid: TestID = {
          aggregationLevel: AggregationLevel.INVOCATION,
          module: '',
          testClass: '',
          method: '',
          // moduleParameters undefined
        };
        expect(testIdentifierHash(invocation, tid)).toBe(
          invocationTestResultHash,
        );
      });
    });
  });

  describe('Helpers', () => {
    it('quote', () => {
      expect(quote('foo')).toBe('"foo"');
      expect(quote('foo"bar')).toBe('"foo\\"bar"');
    });

    it('compoundValue sorting', () => {
      const cv = new CompoundValue('test');
      cv.add(new SimpleValue('b'));
      cv.add(new SimpleValue('a'));
      const expected = '"test"={"a":"b"}';
      expect(cv.serialize()).toBe(expected);
    });
  });
});

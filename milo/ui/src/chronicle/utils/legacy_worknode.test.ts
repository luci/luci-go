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

import { extractLegacyWorkNodeLabel } from './legacy_worknode';

describe('extractLegacyWorkNodeLabel with ported google3 logic', () => {
  it('handles ATP_TEST workExecutorType and testName', () => {
    expect(
      extractLegacyWorkNodeLabel({
        workExecutorType: 'ATP_TEST',
        workParameters: {
          atpTestParameters: { testName: 'CtsOsTestCases' },
        },
      }),
    ).toBe('test CtsOsTestCases');
  });

  it('handles PENDING_CHANGE_BUILD workExecutorType and ReleaseRequest type', () => {
    expect(
      extractLegacyWorkNodeLabel({
        workExecutorType: 'PENDING_CHANGE_BUILD',
        workParameters: {
          releaseRequest: { type: 'CONTINUOUS_INTEGRATION' },
        },
      }),
    ).toBe('build continuousIntegration');
  });

  it('handles custom executor type in camelCase', () => {
    expect(
      extractLegacyWorkNodeLabel({
        workExecutorType: 'ATP_TEST_AGGREGATOR',
        workParameters: {
          atpTestParameters: { testName: 'MyAggregatedTest' },
        },
      }),
    ).toBe('atpTestAggregator MyAggregatedTest');
  });

  it('handles submit queue branch and target', () => {
    expect(
      extractLegacyWorkNodeLabel({
        workExecutorType: 'PENDING_CHANGE_BUILD',
        workParameters: {
          submitQueue: { branch: 'master', target: 'aosp_arm64' },
        },
      }),
    ).toBe('build master:aosp_arm64');
  });

  it('handles image request device, incremental and build', () => {
    expect(
      extractLegacyWorkNodeLabel({
        workExecutorType: 'PENDING_CHANGE_BUILD',
        workParameters: {
          imageRequest: {
            device: 'coral',
            build: { rcName: 'build-123' },
            incrementals: [{ rcName: 'build-100' }],
          },
        },
      }),
    ).toBe('build coral:build-100 \u{2192} build-123');
  });

  it('handles OTA destination', () => {
    expect(
      extractLegacyWorkNodeLabel({
        workExecutorType: 'ATP_TEST',
        workParameters: {
          uploadOtaPackageParameters: {
            gotaDestination: { deploymentName: 'gota-dep' },
          },
        },
      }),
    ).toBe('test GOTA: gota-dep');
  });

  it('handles Klefki platform image workflow request', () => {
    expect(
      extractLegacyWorkNodeLabel({
        workExecutorType: 'ATP_TEST',
        workParameters: {
          klefkiProxyRequest: {
            request: {
              workflowType: 'PLATFORM_IMAGE',
              workflowInputParams: {
                platformImageWorkflowInputParams: {
                  device: 'flame',
                  build: { buildId: '999999' },
                  fromBuilds: [{ buildId: '888888' }],
                },
              },
            },
          },
        },
      }),
    ).toBe('test platformImage flame:888888 \u{2192} 999999');
  });

  it('falls back to custom message with fallback ID if parameters empty', () => {
    expect(
      extractLegacyWorkNodeLabel(
        {
          workExecutorType: 'PENDING_CHANGE_BUILD',
        },
        'my-stage-id',
      ),
    ).toBe('build WorkNode: my-stage-id');
  });
});

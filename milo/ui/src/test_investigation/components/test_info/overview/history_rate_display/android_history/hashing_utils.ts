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
  InvocationID,
  TestID,
  testIdentifierHash,
} from '@/generic_libs/tools/ants_compatibility';
import { RootInvocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

// Interfaces for AnTS properties
// Note: These interfaces are local definitions for custom properties
// found in TestResult.properties within TestVariant.results.
interface Property {
  name: string;
  value: string;
}

interface AntsTestIdentifier {
  module?: string;
  moduleParameters?: Property[];
  testClass?: string;
  method?: string;
  // methodParameters ignored as per user instructions
}

interface AntsTestResultProperties {
  '@type': string;
  antsTestId?: AntsTestIdentifier;
  // other fields ignored
}

export function getSchedulerAndBuildProvider(system: string): {
  scheduler: string;
  buildProvider: string;
} {
  switch (system) {
    case 'pte-stability-collection':
      return { scheduler: 'SPS_PLATFORM', buildProvider: 'androidbuild' };
    case 'atp':
      return { scheduler: 'ATP', buildProvider: 'androidbuild' };
    case 'bazel':
      return { scheduler: 'Bazel', buildProvider: '' };
    case 'ants-relay':
      return { scheduler: 'AnTS Relay', buildProvider: 'androidbuild' };
    case 'ctp':
      return { scheduler: 'CTP', buildProvider: '' };
    case 'mh':
      return { scheduler: 'MH', buildProvider: '' };
    case 'mtt':
      return { scheduler: 'MTT', buildProvider: '' };
    case 'atest':
      return { scheduler: 'atest', buildProvider: '' };
    default:
      return { scheduler: '', buildProvider: 'androidbuild' };
  }
}

export function getAntsTestIdentifierHash(
  rootInvocation: RootInvocation,
  testVariant: TestVariant,
): string | null {
  // 1. Invocation Mapping
  const producerSystem = rootInvocation.definition?.system || '';
  const { scheduler, buildProvider } =
    getSchedulerAndBuildProvider(producerSystem);

  const invID: InvocationID = {
    testName: rootInvocation.definition?.name || '',
    properties: rootInvocation.definition?.properties?.def || {},
    // Use primaryBuild for both branch and target for consistency
    branch: rootInvocation.primaryBuild?.androidBuild?.branch || '',
    target: rootInvocation.primaryBuild?.androidBuild?.buildTarget || '',
    scheduler,
    buildProvider,
  };

  // 2. Test ID Mapping
  let antsTestId: AntsTestIdentifier | undefined;

  for (const bundle of testVariant.results) {
    const props = bundle.result?.properties;
    if (
      props &&
      props['@type'] ===
        'type.googleapis.com/wireless.android.busytown.proto.TestResultProperties'
    ) {
      const typedProps = props as unknown as AntsTestResultProperties;
      if (typedProps.antsTestId) {
        antsTestId = typedProps.antsTestId;
        break;
      }
    }
  }
  if (!antsTestId) {
    return null;
  }
  const testID: TestID = {
    module: antsTestId.module || '',
    testClass: antsTestId.testClass || '',
    method: antsTestId.method || '',
    aggregationLevel: AggregationLevel.METHOD,
    moduleParameters: antsTestId.moduleParameters?.reduce(
      (acc, p) => {
        acc[p.name] = p.value;
        return acc;
      },
      {} as Record<string, string>,
    ),
  };

  return testIdentifierHash(invID, testID);
}

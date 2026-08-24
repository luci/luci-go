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
import {
  AnyInvocation,
  isRootInvocation,
} from '@/test_investigation/utils/invocation_utils';
import { ANDROID_BUILD_CORP_HOST } from '@/test_investigation/utils/test_info_utils';

import { getInvocationTag } from '../../../utils/test_info_utils';
import { FORREST_CREATE_URL, VIRTUAL_TARGET_PREFIXES } from '../constants';

export interface ForrestQueryParams {
  build_type: string;
  run_type: string;
  test_name?: string;
  build_targets?: string;
  build_id?: string;
  atest_command?: string;
  product?: string;
  [key: string]: string | undefined;
}

export interface BuildTarget {
  buildTarget?: string;
  androidBuild?: {
    buildTarget?: string;
  };
}

export interface AndroidBuild {
  buildId: string;
  buildTarget: string;
  branch?: string;
}

export const SAFE_BRANCH_REGEX = /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/;
export const SAFE_TARGET_REGEX = /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/;
export const SAFE_BUILD_ID_REGEX = /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/;
export const SAFE_ABI_REGEX = /^[a-zA-Z0-9_-]+$/;

/**
 * Determines if the target passed can be booted on acloud or not.
 */
export function isVirtualTarget(
  targetName: string | null | undefined,
): boolean {
  if (!targetName || !SAFE_TARGET_REGEX.test(targetName)) {
    return false;
  }

  return VIRTUAL_TARGET_PREFIXES.some((prefix) =>
    targetName.startsWith(prefix),
  );
}

export function escapeShellArg(arg: string | undefined): string {
  if (!arg || arg.length === 0) return "''";
  // Wraps in single quotes and escapes any internal single quotes
  return "'" + arg.replace(/'/g, "'\\''") + "'";
}

export function getAtestCommand(
  testVariant: OutputTestVerdict,
  params?: {
    moduleOnly?: boolean;
    omitAtest?: boolean;
    omitExtraArgs?: boolean;
    acloudBuild?: AndroidBuild;
  },
): string | null {
  const moduleName = testVariant?.testIdStructured?.moduleName;
  const testIdStructured = testVariant?.testIdStructured || undefined;
  if (!moduleName) {
    return null;
  }

  let testIdentifier = moduleName;
  if (!params?.moduleOnly) {
    const testClass = `${testIdStructured?.coarseName ?? ''}.${testIdStructured?.fineName ?? ''}`;
    const testMethod = testIdStructured?.caseName ?? '';
    if (testClass !== '' || testMethod !== '') {
      if (testClass !== '') {
        testIdentifier += `:${testClass}`;
      }
      if (testMethod !== '') {
        testIdentifier += `#${testMethod}`;
      }
    }
  }
  const escapedIdentifier = escapeShellArg(testIdentifier);

  let command = `${(params?.omitAtest ?? false) ? '' : 'atest '}${escapedIdentifier}`;

  const build = params?.acloudBuild;
  if (build) {
    if (
      !build.branch ||
      !build.buildTarget ||
      !build.buildId ||
      !SAFE_BRANCH_REGEX.test(build.branch) ||
      !SAFE_TARGET_REGEX.test(build.buildTarget) ||
      !SAFE_BUILD_ID_REGEX.test(build.buildId) ||
      !isVirtualTarget(build.buildTarget)
    ) {
      return null;
    }
    const acloudArgs = `--branch ${build.branch} --build-target ${build.buildTarget} --build-id ${build.buildId}`;
    command = `${command} --acloud-create ${escapeShellArg(acloudArgs)}`;
  }

  if (!params?.omitExtraArgs) {
    const extraArgs: string[] = [];
    const moduleAbi = testVariant.variant?.def['module_abi'];
    if (moduleAbi && SAFE_ABI_REGEX.test(moduleAbi)) {
      extraArgs.push(`--abi ${escapeShellArg(moduleAbi)}`);
    }
    if (testVariant.variant?.def['module_param'] === 'instant') {
      extraArgs.push('--instant');
    }
    if (extraArgs.length > 0) {
      command = `${command} -- ${extraArgs.join(' ')}`;
    }
  }
  return command;
}

export function getForrestLink(
  invocation: AnyInvocation,
  atestCommand?: string,
): string {
  const buildTargets = [];
  const primaryBuild = isRootInvocation(invocation)
    ? invocation?.primaryBuild?.androidBuild
    : invocation?.properties?.primaryBuild;
  buildTargets.push(primaryBuild.buildTarget);
  const extraBuildTargets = isRootInvocation(invocation)
    ? (invocation.extraBuilds ?? []).map(
        (build) => build?.androidBuild?.buildTarget ?? undefined,
      )
    : (invocation.properties?.extraBuilds ?? []).map(
        (extraBuild: BuildTarget) => extraBuild?.buildTarget ?? undefined,
      );

  buildTargets.push(...extraBuildTargets);
  buildTargets.filter((target): target is string => target !== null);

  const testName =
    atestCommand === null &&
    getInvocationTag(invocation.tags, 'scheduler') === 'ATP'
      ? (invocation.name ?? undefined)
      : undefined;

  const product =
    atestCommand !== null &&
    !isVirtualTarget(primaryBuild.buildTarget ?? undefined)
      ? getInvocationTag(invocation.tags, 'run_target')
      : undefined;
  const params: ForrestQueryParams = {
    build_type: 'build',
    run_type: 'test',
    test_name: testName,
    build_targets: buildTargets.length > 0 ? buildTargets.join(',') : undefined,
    build_id: primaryBuild?.buildId ?? undefined,
    atest_command: atestCommand,
    product,
  };
  const paramBuilder = new URLSearchParams();
  for (const key of Object.keys(params)) {
    if (params[key]) {
      paramBuilder.append(key, params[key]);
    }
  }

  return `${ANDROID_BUILD_CORP_HOST}${FORREST_CREATE_URL}?${paramBuilder.toString()}`;
}

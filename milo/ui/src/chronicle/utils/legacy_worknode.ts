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

/**
 * This file contains helper functions for generating labels from legacy WorkNode
 * parameters. This code is largely copied from the existing Workplan Viewer code
 * at http://shortn/_T8wYkFezwF.
 */

import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

export const TYPE_URL_LEGACY_WORKNODE_STAGE =
  'type.googleapis.com/wireless.android.launchcontrol.WorkNodeStage';
export const TYPE_URL_LEGACY_WORKNODE =
  'type.googleapis.com/wireless.android.launchcontrol.WorkNode';
export const TYPE_URL_LEGACY_WORKPRODUCT =
  'type.googleapis.com/wireless.android.launchcontrol.WorkProduct';

const ARROW = '\u{2192}'; // →

export interface LegacyReleaseRequest {
  readonly type?: string;
}

export interface LegacyAtpTestParameters {
  readonly testName?: string;
}

export interface LegacySuiteParameters {
  readonly testName?: string;
}

export interface LegacyTestProviderParameters {
  readonly suiteParameters?: LegacySuiteParameters;
}

export interface LegacySubmitQueue {
  readonly branch?: string;
  readonly target?: string;
}

export interface LegacyImageBuild {
  readonly rcName?: string;
  readonly buildId?: string;
}

export interface LegacyImageRequest {
  readonly device?: string;
  readonly build?: LegacyImageBuild;
  readonly incrementals?: readonly LegacyImageBuild[];
}

export interface LegacyOtaDestination {
  readonly deploymentName?: string;
}

export interface LegacyAthenaDestination {
  readonly companyId?: string;
}

export interface LegacyUploadOtaPackageParameters {
  readonly gotaDestination?: LegacyOtaDestination;
  readonly athenaDestination?: LegacyAthenaDestination;
}

export interface LegacyKlefkiBuild {
  readonly rcName?: string;
  readonly buildId?: string;
}

export interface LegacyPlatformImageWorkflowInputParams {
  readonly device?: string;
  readonly build?: LegacyKlefkiBuild;
  readonly fromBuilds?: readonly LegacyKlefkiBuild[];
}

export interface LegacyAutoOtaWorkflowInputParams {
  readonly deviceName?: string;
  readonly toBuild?: LegacyKlefkiBuild;
  readonly fromBuilds?: readonly LegacyKlefkiBuild[];
}

export interface LegacyDeviceOtaWorkflowInputParams {
  readonly device?: string;
  readonly build?: LegacyKlefkiBuild;
  readonly fromBuilds?: readonly LegacyKlefkiBuild[];
}

export interface LegacyWorkflowInputParams {
  readonly platformImageWorkflowInputParams?: LegacyPlatformImageWorkflowInputParams;
  readonly autoOtaWorkflowInputParams?: LegacyAutoOtaWorkflowInputParams;
  readonly deviceOtaWorkflowInputParams?: LegacyDeviceOtaWorkflowInputParams;
}

export interface LegacyStartWorkflowRequest {
  readonly workflowType?: string;
  readonly workflowInputParams?: LegacyWorkflowInputParams;
}

export interface LegacyKlefkiProxyRequest {
  readonly request?: LegacyStartWorkflowRequest;
}

export interface LegacyWorkParameters {
  readonly releaseRequest?: LegacyReleaseRequest;
  readonly atpTestParameters?: LegacyAtpTestParameters;
  readonly testProviderParameters?: LegacyTestProviderParameters;
  readonly submitQueue?: LegacySubmitQueue;
  readonly imageRequest?: LegacyImageRequest;
  readonly uploadOtaPackageParameters?: LegacyUploadOtaPackageParameters;
  readonly klefkiProxyRequest?: LegacyKlefkiProxyRequest;
}

export interface LegacyWorkNode {
  readonly workExecutorType?: string | number;
  readonly workParameters?: LegacyWorkParameters;
  // We use unknown here as we don't have access to the proto definitions. We also
  // just show the output as raw JSON and don't access any inner fields directly.
  readonly workOutput?: unknown;
}

function toCamelCase(str: string): string {
  return str
    .toLowerCase()
    .replace(/_([a-z])/g, (_, letter) => letter.toUpperCase());
}

function getWorkExecutorTypeName(executorType?: string | number): string {
  if (!executorType) {
    return 'unknown';
  }
  const typeStr = String(executorType).trim().toUpperCase();
  if (typeStr === 'PENDING_CHANGE_BUILD') {
    return 'build';
  }
  if (typeStr === 'ATP_TEST') {
    return 'test';
  }
  return toCamelCase(typeStr);
}

function getReleaseRequestTypeName(typeStr?: string): string {
  if (!typeStr) return '';
  return toCamelCase(typeStr);
}

function getWorkflowTypeName(typeStr?: string): string {
  if (!typeStr) return 'unknown';
  if (typeStr === 'WORKFLOW_TYPE_UNSPECIFIED') return 'unknown';
  return toCamelCase(typeStr);
}

function getPlatformImageName(
  klefkiRequest: LegacyStartWorkflowRequest,
  typeName: string,
): string {
  const input =
    klefkiRequest.workflowInputParams?.platformImageWorkflowInputParams;
  const device = input?.device ?? '';
  const build = input?.build?.rcName || input?.build?.buildId || '';
  const fromBuilds = input?.fromBuilds;
  const incremental = fromBuilds?.length
    ? `${fromBuilds[0].rcName || fromBuilds[0].buildId || ''} ${ARROW} `
    : '';
  return `${typeName} ${device}:${incremental}${build}`;
}

function getAutoOtaName(
  klefkiRequest: LegacyStartWorkflowRequest,
  typeName: string,
): string {
  const input = klefkiRequest.workflowInputParams?.autoOtaWorkflowInputParams;
  const device = input?.deviceName ?? '';
  const build = input?.toBuild?.rcName || input?.toBuild?.buildId || '';
  const fromBuilds = input?.fromBuilds;
  const incremental = fromBuilds?.length
    ? `${fromBuilds[0].rcName || fromBuilds[0].buildId || ''} ${ARROW} `
    : '';
  return `${typeName} ${device}:${incremental}${build}`;
}

function getDeviceOtaName(
  klefkiRequest: LegacyStartWorkflowRequest,
  typeName: string,
): string {
  const input = klefkiRequest.workflowInputParams?.deviceOtaWorkflowInputParams;
  const device = input?.device ?? '';
  const build = input?.build?.rcName || input?.build?.buildId || '';
  const fromBuilds = input?.fromBuilds;
  const incremental = fromBuilds?.length
    ? `${fromBuilds[0].rcName || fromBuilds[0].buildId || ''} ${ARROW} `
    : '';
  return `${typeName} ${device}:${incremental}${build}`;
}

function nodeName(node: LegacyWorkNode): string {
  const params = node.workParameters;
  if (!params) {
    return '';
  }

  const releaseType = params.releaseRequest?.type;
  if (releaseType !== undefined) {
    return getReleaseRequestTypeName(releaseType) || releaseType;
  }

  const atpTestName = params.atpTestParameters?.testName;
  if (atpTestName) {
    return atpTestName;
  }

  const testParams = params.testProviderParameters;
  if (testParams) {
    return testParams.suiteParameters?.testName ?? '';
  }

  const submitQueue = params.submitQueue;
  if (submitQueue) {
    return `${submitQueue.branch || ''}:${submitQueue.target || ''}`;
  }

  const imageRequest = params.imageRequest;
  if (imageRequest) {
    const build =
      imageRequest.build?.rcName || imageRequest.build?.buildId || '';
    const firstInc = imageRequest.incrementals?.[0];
    const incremental = firstInc?.rcName || firstInc?.buildId || '';
    return (
      `${imageRequest.device || ''}:` +
      `${incremental ? `${incremental} ${ARROW} ` : ''}${build}`
    );
  }

  const otaParams = params.uploadOtaPackageParameters;
  if (otaParams) {
    if (otaParams.gotaDestination) {
      return `GOTA: ${otaParams.gotaDestination.deploymentName || ''}`;
    }
    if (otaParams.athenaDestination) {
      return `Athena: ${otaParams.athenaDestination.companyId || ''}`;
    }
  }

  const klefkiRequest = params.klefkiProxyRequest?.request;
  if (klefkiRequest) {
    const klefkiWorkflowType = klefkiRequest.workflowType;
    const typeName = getWorkflowTypeName(klefkiWorkflowType);
    const cleanType = String(klefkiWorkflowType).toUpperCase().trim();
    switch (cleanType) {
      case 'PLATFORM_IMAGE':
        return getPlatformImageName(klefkiRequest, typeName);
      case 'DEVICE_OTA':
        return getDeviceOtaName(klefkiRequest, typeName);
      case 'AUTO_OTA':
        return getAutoOtaName(klefkiRequest, typeName);
      default:
        return typeName;
    }
  }

  return '';
}

export function extractLegacyWorkNodeLabel(
  workNode?: LegacyWorkNode | unknown,
  fallbackId?: string,
): string | undefined {
  if (!workNode || typeof workNode !== 'object') {
    return undefined;
  }
  const node = workNode as LegacyWorkNode;

  const execTypeName = getWorkExecutorTypeName(node.workExecutorType);
  const name = nodeName(node);

  if (name) {
    return `${execTypeName} ${name}`;
  }

  // Fallback if nodeName is empty or unable to be parsed
  if (fallbackId) {
    return `${execTypeName} WorkNode: ${fallbackId}`;
  }

  return undefined;
}

export function extractLegacyWorkNode(
  stage: Stage,
  valueDataMap: Map<string, ValueData> | ReadonlyMap<string, ValueData>,
): LegacyWorkNode | undefined {
  const legacyWorkNodeRef = stage.legacy?.worknode;
  if (!legacyWorkNodeRef?.digest) {
    return undefined;
  }
  const valueData = valueDataMap.get(legacyWorkNodeRef.digest);
  const jsonStr = valueData?.json?.value;
  if (!jsonStr) {
    return undefined;
  }
  try {
    return JSON.parse(jsonStr) as LegacyWorkNode;
  } catch {
    return undefined;
  }
}

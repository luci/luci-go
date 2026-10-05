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

import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { getNodeSearchIndex } from './check_utils';

export interface SearchIndexWorkerNode {
  id: string;
  label: string;
  view?: Check | Stage;
}

export interface SearchIndexWorkerRequest {
  nodes: SearchIndexWorkerNode[];
  valueDataMap?: ReadonlyMap<string, ValueData>;
}

export interface SearchIndexWorkerResponse {
  searchIndexMap?: Map<string, string>;
  error?: unknown;
}

export function buildSearchIndexMap(
  nodes: SearchIndexWorkerNode[],
  valueDataMap?: ReadonlyMap<string, ValueData>,
): Map<string, string> {
  const searchIndexMap = new Map<string, string>();
  for (const node of nodes) {
    searchIndexMap.set(
      node.id,
      getNodeSearchIndex(node.id, node.label, node.view, valueDataMap),
    );
  }
  return searchIndexMap;
}

self.onmessage = (e: MessageEvent<SearchIndexWorkerRequest>) => {
  const { nodes, valueDataMap } = e.data;
  try {
    const searchIndexMap = buildSearchIndexMap(nodes, valueDataMap);
    self.postMessage({ searchIndexMap } satisfies SearchIndexWorkerResponse);
  } catch (error) {
    self.postMessage({ error } satisfies SearchIndexWorkerResponse);
  }
};

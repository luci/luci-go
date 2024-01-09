// Copyright 2024 The LUCI Authors.
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

import { AlertJson, BugId, TreeJson } from '@/monitoring/util/server_json';

const monorailProjectId = (tree: TreeJson): string => {
  return tree.default_monorail_project_name || 'chromium';
};

export const linkBug = async (
  tree: TreeJson,
  alerts: AlertJson[],
  bugId: string,
): Promise<void> => {
  const data = {
    bugs: [
      {
        id: bugId,
        projectId: monorailProjectId(tree),
      },
    ],
  };

  // TODO: This API doesn't support CORS, so this doesn't currently work.  Create an RPC to do the same thing.
  const promises = alerts.map((alert) =>
    post('/api/v1/annotations/' + tree.name + '/add', {
      ...data,
      key: alert.key,
    }),
  );
  await Promise.all(promises);
};

export const unlinkBug = async (
  tree: TreeJson,
  alert: AlertJson,
  bugs: BugId[],
): Promise<void> => {
  // TODO: This API doesn't support CORS, so this doesn't currently work.  Create an RPC to do the same thing.
  await post('/api/v1/annotations/' + tree.name + '/remove', {
    bugs,
    key: alert.key,
  });
};

const post = async <DataType, ResponseType>(
  path: string,
  data: DataType,
): Promise<ResponseType> => {
  const response = await fetch(path, {
    method: 'POST',
    credentials: 'include',
    body: JSON.stringify({
      data,
    }),
  });
  if (!response.ok) {
    throw new Error(await response.text());
  }
  return response.json();
};

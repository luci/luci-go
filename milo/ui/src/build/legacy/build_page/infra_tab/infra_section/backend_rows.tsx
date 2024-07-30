// Copyright 2023 The LUCI Authors.
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

import { Link } from '@mui/material';
import { JSONPath as jsonpath } from 'jsonpath-plus';

import { BuildStatusIcon } from '@/build/components/build_status_icon';
import { OutputBuildInfra_Backend } from '@/build/types';
import { getBotUrl } from '@/swarming/tools/utils';

export interface BackendRowsProps {
  readonly backend: OutputBuildInfra_Backend;
}

export function BackendRows({ backend }: BackendRowsProps) {
  const task = backend.task;

  const botId = backend.task.id.target.startsWith('swarming://')
    ? jsonpath<string | undefined>({
        json: task.details || {},
        path: '$.bot_dimensions.id[0]@string()',
        wrap: false,
      })
    : undefined;
  const serviceAccount = jsonpath<string | undefined>({
    json: backend.config || {},
    path: '$.service_account@string()',
    wrap: false,
  });

  return (
    <>
      <tr>
        <td>Backend Target:</td>
        <td>{task.id.target}</td>
      </tr>
      <tr>
        <td>Backend Task:</td>
        <td>
          <BuildStatusIcon status={task.status} />
          {task.link ? (
            <Link href={task.link} target="_blank" rel="noopener">
              {task.id.id}
            </Link>
          ) : (
            task.id.id
          )}
        </td>
      </tr>
      {botId && (
        <tr>
          <td>Backend Bot:</td>
          <td>
            <Link
              href={getBotUrl(backend.hostname, botId)}
              target="_blank"
              rel="noopenner"
            >
              {botId}
            </Link>
          </td>
        </tr>
      )}
      {serviceAccount && (
        <tr>
          <td>Service Account:</td>
          <td>{serviceAccount}</td>
        </tr>
      )}
    </>
  );
}

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

import { Link, styled } from '@mui/material';

import { getGitilesCommitURL } from '@/gitiles/tools/utils';
import {
  GitilesRef,
  Variant,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';

const Table = styled('table')`
  padding-left: 5px;
  & > tbody > tr > td:nth-of-type(1) {
    font-weight: bold;
  }
  & > tbody > tr > td:nth-of-type(2) {
    word-break: break-word;
    font-weight: 400;
  }
`;

const KeyValueSpan = styled('span')`
  & > span:first-of-type {
    color: var(--greyed-out-text-color);
  }

  &:not(:first-of-type) {
    margin-left: 5px;
  }
  &:not(:last-of-type)::after {
    content: ',';
    color: var(--greyed-out-text-color);
  }
`;

export interface TestVariantBranchIdProps {
  readonly gitilesRef: GitilesRef;
  readonly testId: string;
  readonly variant?: Variant;
}

export function TestVariantBranchId({
  gitilesRef,
  testId,
  variant,
}: TestVariantBranchIdProps) {
  return (
    <Table>
      <tbody>
        <tr>
          <td width="1px">Branch:</td>
          <td>
            <Link href={getGitilesCommitURL(gitilesRef)}>{gitilesRef.ref}</Link>
          </td>
        </tr>
        <tr>
          <td width="1px">Test ID:</td>
          <td>{testId}</td>
        </tr>
        <tr>
          <td width="1px">Variant:</td>
          <td>
            {Object.entries(variant?.def || {}).map(([k, v]) => (
              <KeyValueSpan key={k}>
                <span>{k}: </span>
                <span>{v}</span>
              </KeyValueSpan>
            ))}
          </td>
        </tr>
      </tbody>
    </Table>
  );
}

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

import { useLogGroupListState, useSearchFilter } from '../context';
import { FormData } from '../form_data';
import { VariantLine } from '../variant_line';

import { LogDialogBase } from './log_dialog_base';
import { TestLogList } from './test_log_list';

export interface TestLogDialogProps {
  readonly project: string;
}

export function TestLogDialog({ project }: TestLogDialogProps) {
  const state = useLogGroupListState();
  const filter = useSearchFilter();
  const { testLogGroupIdentifier } = state;
  if (!testLogGroupIdentifier) {
    return <></>;
  }
  const { testID, variant, artifactID } = testLogGroupIdentifier;
  return (
    <LogDialogBase
      dialogHeader={
        <table>
          <tbody>
            <tr>
              <td width="1px" style={{ whiteSpace: 'nowrap' }}>
                Test ID:
              </td>
              <td css={{ fontWeight: 400 }}>{testID}</td>
            </tr>
            <tr>
              <td width="1px" style={{ whiteSpace: 'nowrap' }}>
                Variant:
              </td>
              <td css={{ fontWeight: 400 }}>
                {variant && <VariantLine variant={variant} />}
              </td>
            </tr>
            <tr>
              <td width="1px" style={{ whiteSpace: 'nowrap' }}>
                Log file:
              </td>
              <td css={{ fontWeight: 400 }}>{artifactID}</td>
            </tr>
          </tbody>
        </table>
      }
    >
      {filter && (
        <TestLogList
          project={project}
          logGroupIdentifer={testLogGroupIdentifier}
          searchString={FormData.getSearchString(filter.form)}
          startTime={filter.startTime.toString()}
          endTime={filter.endTime.toString()}
        />
      )}
    </LogDialogBase>
  );
}

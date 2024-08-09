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

import { DateTime } from 'luxon';

import { useLogGroupListState } from '../contexts';
import { FormData } from '../form_data';
import { VariantLine } from '../variant_line';

import { InvocationLogList } from './invocation_log_list';
import { LogDialogBase } from './log_dialog_base';

export interface InvocationLogDialogProps {
  readonly project: string;
  readonly form: FormData;
  readonly startTime: DateTime;
  readonly endTime: DateTime;
}

export function InvocationLogDialog({
  project,
  form,
  startTime,
  endTime,
}: InvocationLogDialogProps) {
  const { invocationLogGroupIdentifier } = useLogGroupListState();
  if (!invocationLogGroupIdentifier) {
    return <></>;
  }
  const { variantUnion, artifactID } = invocationLogGroupIdentifier;
  return (
    <LogDialogBase
      dialogHeader={
        <table>
          <tbody>
            <tr>
              <td width="1px" style={{ whiteSpace: 'nowrap' }}>
                Variant union:
              </td>
              <td css={{ fontWeight: 400 }}>
                {variantUnion && <VariantLine variant={variantUnion} />}
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
      <InvocationLogList
        project={project}
        logGroupIdentifer={invocationLogGroupIdentifier}
        searchString={FormData.getSearchString(form)}
        startTime={startTime.toString()}
        endTime={endTime.toString()}
      />
    </LogDialogBase>
  );
}

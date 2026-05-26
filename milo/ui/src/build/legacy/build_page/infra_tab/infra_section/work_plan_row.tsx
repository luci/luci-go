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

import { fromString } from '@/chronicle/utils/id/from_string';
import { root } from '@/chronicle/utils/id/wrap';
import { MiloLink } from '@/common/components/link';
import { Link as LinkData } from '@/common/models/link';
import { BuildInfra_TurboCI } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';

export interface WorkPlanRowProps {
  readonly turboci?: BuildInfra_TurboCI;
}

export function WorkPlanRow({ turboci }: WorkPlanRowProps) {
  let link: LinkData | null = null;

  if (turboci?.stageAttemptId) {
    try {
      const parsed = fromString(turboci.stageAttemptId);
      const { wp, stage } = root(parsed);
      if (wp?.id && stage?.id) {
        const lastColonIdx = turboci.stageAttemptId.lastIndexOf(':');
        const labelText =
          lastColonIdx !== -1
            ? turboci.stageAttemptId.substring(0, lastColonIdx)
            : turboci.stageAttemptId;

        const attemptId = parsed.stageAttempt?.idx;
        link = {
          label: labelText,
          url: `/ui/chronicle/${wp.id}/graph?nodeId=${encodeURIComponent(stage.id)}`,
          ariaLabel: `graph view for attempt ${attemptId} of stage ${stage.id} in workplan ${wp.id}`,
        };
      }
    } catch (_e) {
      // Fallback to "None" if parsing fails.
    }
  }

  return (
    <tr>
      <td>WorkPlan:</td>
      <td>{link ? <MiloLink link={link} target="_blank" /> : 'None'}</td>
    </tr>
  );
}

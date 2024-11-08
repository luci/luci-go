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

import { useQuery } from '@tanstack/react-query';

import { useRulesService } from '@/clusters/services/services';
import { prpcRetrier } from '@/clusters/tools/prpc_retrier';
import { ListRulesRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

const useFetchRules = (project: string) => {
  const rulesService = useRulesService();

  return useQuery(
    ['rules', project],
    async () => {
      const request: ListRulesRequest = {
        parent: `projects/${encodeURIComponent(project || '')}`,
      };

      const response = await rulesService.List(request);

      const rules = response.rules || [];
      const sortedRules = [...rules].sort((a, b) => {
        // These are RFC 3339-formatted date/time strings.
        // Because they are all use the same timezone, and RFC 3339
        // date/times are specified from most significant to least
        // significant, any string sort that produces a lexicographical
        // ordering should also sort by time.

        // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
        return b.lastAuditableUpdateTime!.localeCompare(
          a.lastAuditableUpdateTime!,
        );
      });
      return sortedRules;
    },
    {
      retry: prpcRetrier,
    },
  );
};

export default useFetchRules;

// Copyright 2022 The LUCI Authors.
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

const useFetchRule = (project: string, ruleId: string) => {
  const rulesService = useRulesService();

  return useQuery({
    queryKey: ['rules', project, ruleId],

    queryFn: async () =>
      await rulesService.Get({
        name: `projects/${project}/rules/${ruleId}`,
      }),

    retry: prpcRetrier,
  });
};

export default useFetchRule;

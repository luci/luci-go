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

import { useQuery } from 'react-query';

import { getRulesService } from '@/legacy_services/rules';
import { prpcRetrier } from '@/legacy_services/shared_models';

const useFetchRule = (ruleId: string | undefined, project: string | undefined) => {
  const rulesService = getRulesService();

  return useQuery(['rule', project, ruleId], async () => await rulesService.get(
      {
        name: `projects/${project}/rules/${ruleId}`,
      },
  ), {
    retry: prpcRetrier,
  });
};

export default useFetchRule;

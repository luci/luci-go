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

import { getRecipeLink } from '@/build/tools/build_utils';

import { useBuild } from '../../hooks';

import { BackendRows } from './backend_rows';
import { BotLinkRow } from './bot_link_row';
import { BuildbucketRow } from './buildbucket_row';
import { InvocationRow } from './invocation_row';
import { RecipeRow } from './recipe_row';
import { ServiceAccountRow } from './service_account_row';
import { SwarmingTaskRow } from './swarming_task_row';

export function InfraSection() {
  const build = useBuild();
  if (!build) {
    return <></>;
  }

  const recipeLink = getRecipeLink(build);

  return (
    <>
      <h3>Infra</h3>
      <table
        css={{
          '& td:nth-of-type(2)': {
            clear: 'both',
            overflowWrap: 'anywhere',
          },
        }}
      >
        <tbody>
          <BuildbucketRow buildId={build.id} />
          {build.infra?.backend && (
            <BackendRows backend={build.infra.backend} />
          )}
          {build.infra?.swarming && (
            <>
              <SwarmingTaskRow swarming={build.infra.swarming} />
              <BotLinkRow swarming={build.infra.swarming} />
              <ServiceAccountRow swarming={build.infra.swarming} />
            </>
          )}
          {recipeLink && <RecipeRow recipeLink={recipeLink} />}
          <InvocationRow resultdb={build.infra?.resultdb} />
        </tbody>
      </table>
    </>
  );
}

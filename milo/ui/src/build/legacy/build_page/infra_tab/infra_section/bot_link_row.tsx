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

import { MiloLink } from '@/common/components/link';
import { getBotLink } from '@/common/tools/build_utils';
import { BuildInfra_Swarming } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { StringPair } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export interface BotLinkRowProps {
  readonly swarming: BuildInfra_Swarming;
}

export function BotLinkRow({ swarming }: BotLinkRowProps) {
  const botLink = swarming
    ? getBotLink({
        botDimensions:
          swarming.botDimensions.map((d) => StringPair.fromPartial(d)) || [],
        hostname: swarming.hostname,
      })
    : null;

  return (
    <tr>
      <td>Bot:</td>
      <td>{botLink ? <MiloLink link={botLink} target="_blank" /> : 'N/A'}</td>
    </tr>
  );
}

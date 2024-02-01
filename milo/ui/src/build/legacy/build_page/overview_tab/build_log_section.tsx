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

import { observer } from 'mobx-react-lite';

import { BuildbucketLogLink } from '@/common/components/buildbucket_log_link';
import { useStore } from '@/common/store';

export const BuildLogSection = observer(() => {
  const store = useStore();
  const logs = store.buildPage.build?.data.output?.logs;
  if (!logs) {
    return <></>;
  }

  return (
    <>
      <h3>Build Logs</h3>
      <ul>
        {logs.map((log) => (
          <li key={log.name}>
            <BuildbucketLogLink log={log} />
          </li>
        ))}
      </ul>
    </>
  );
});

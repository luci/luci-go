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

import { Typography } from '@mui/material';
import { observer } from 'mobx-react-lite';
import { useEffect } from 'react';

import '../../components/dot_spinner';
import { DotSpinner } from '../../components/dot_spinner';
import { useStore } from '../../store';

export const TestList = observer(() => {
  const { testProject, searchQuery, testLoader } = useStore().searchPage;

  useEffect(() => {
    testLoader?.loadFirstPage();
  }, [testLoader]);

  if (!searchQuery) {
    return <></>;
  }

  if (!testLoader?.loadedFirstPage) {
    return (
      <Typography component="span">
        Loading
        <DotSpinner />
      </Typography>
    );
  }

  return (
    <>
      <ul>
        {testLoader.items.map((testId) => (
          <li key={testId}>
            <a
              href={`/ui/test/${encodeURIComponent(
                testProject
              )}/${encodeURIComponent(testId)}`}
              target="_blank"
              rel="noreferrer"
            >
              {testId}
            </a>
          </li>
        ))}
      </ul>
      {testLoader?.isLoading ?? true ? (
        <Typography component="span">
          Loading
          <DotSpinner />
        </Typography>
      ) : (
        !testLoader.loadedAll && (
          <Typography
            component="span"
            className="active-text"
            onClick={() => testLoader?.loadNextPage()}
          >
            [load more]
          </Typography>
        )
      )}
    </>
  );
});

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

import Link from '@mui/material/Link';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';

import { Culprit } from '@/monitoring/util/server_json';

import { generateAnalysisUrl } from './util';

interface CulpritSectionProps {
  culprits: Culprit[];
  failed_bbid: string;
}

export const CulpritSection = ({
  culprits,
  failed_bbid,
}: CulpritSectionProps) => {
  return (
    <>
      <Link
        href={generateAnalysisUrl(failed_bbid)}
        target="_blank"
        rel="noopener"
      >
        LUCI Bisection
      </Link>
      &nbsp; has identified the following CL(s) as culprit(s) of the failure:
      <List>
        {culprits.map((c) => (
          <ListItem key={c.review_url}>
            <Link
              href={c.review_url}
              target="_blank"
              rel="noopener"
              onClick={() => {
                ga('send', {
                  hitType: 'event',
                  eventCategory: 'LuciBisection',
                  eventAction: 'ClickCulpritLink',
                  eventLabel: c.review_url,
                  transport: 'beacon',
                });
              }}
            >
              {getCulpritDisplayUrl(c)}
            </Link>
          </ListItem>
        ))}
      </List>
    </>
  );
};

function getCulpritDisplayUrl(c: Culprit) {
  if (c.review_title == '' || c.review_title == null) {
    return c.review_url;
  }
  return c.review_title;
}

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

import { Link } from '@mui/material';

import { GerritChange } from '@/common/services/buildbucket';
import { getGerritChangeURL } from '@/common/tools/url_utils';

interface Props {
  readonly cl: GerritChange;
}

export function GerritClLink({ cl }: Props) {
  return (
    <Link key={cl.change} href={getGerritChangeURL(cl)}>
      CL {cl.change} (ps #{cl.patchset})
    </Link>
  );
}

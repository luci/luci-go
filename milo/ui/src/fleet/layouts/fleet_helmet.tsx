// Copyright 2025 The LUCI Authors.
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

import { Helmet } from 'react-helmet';

import bassFavicon from '@/common/assets/favicons/bass-32.png';

interface FleetHelmetProps {
  pageTitle: string;
}

/**
 * Shared component for common <Helmet /> configs like page title.
 */
export function FleetHelmet({ pageTitle }: FleetHelmetProps) {
  return (
    <Helmet>
      <title>{pageTitle}</title>
      <link rel="icon" href={bassFavicon} />
    </Helmet>
  );
}

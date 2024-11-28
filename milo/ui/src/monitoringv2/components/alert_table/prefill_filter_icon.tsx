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

import SearchIcon from '@mui/icons-material/Search';
import { Tooltip } from '@mui/material';

import { useFilterQuery } from '../alerts/hooks';

interface PrefillFilterIconProps {
  filter: string;
}

/**
 * An PrefillFilterIcon shows a search icon that sets the page filter to the given value when clicked.
 */
export const PrefillFilterIcon = ({ filter }: PrefillFilterIconProps) => {
  const [_, updateFilter] = useFilterQuery();
  return (
    <Tooltip title="show matching alerts">
      <SearchIcon
        fontSize="small"
        color="primary"
        sx={{ cursor: 'pointer' }}
        onClick={(e: React.MouseEvent<SVGElement>): void => {
          e.stopPropagation();
          updateFilter(filter);
        }}
      />
    </Tooltip>
  );
};

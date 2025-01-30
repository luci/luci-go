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

import TabContext from '@mui/lab/TabContext';
import TabList from '@mui/lab/TabList';
import TabPanel from '@mui/lab/TabPanel';
import Box from '@mui/material/Box';
import Tab from '@mui/material/Tab';
import Typography from '@mui/material/Typography';

import { GroupForm } from '@/authdb/components/group_form';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

interface GroupDetailsProps {
  name: string;
  refetchList: (fresh: boolean) => void;
}

export function GroupDetails({ name, refetchList }: GroupDetailsProps) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const handleTabChange = (newValue: string) => {
    setSearchParams(
      (params) => {
        params.set('tab', newValue);
        return params;
      },
      { replace: true },
    );
  };

  const validValues = ['overview', 'permissions', 'listing'];
  let value = searchParams.get('tab');
  if (!value || validValues.indexOf(value) === -1) {
    value = 'overview';
  }

  function a11yProps(value: string) {
    return {
      id: `tab-${value}`,
      'aria-controls': `tabpanel-${value}`,
      value: `${value}`,
    };
  }

  return (
    <>
      <TabContext value={value}>
        <Box sx={{ width: '100%' }}>
          <Typography variant="h4" sx={{ pl: 2, pb: 0.5, pt: 2 }}>
            {name}
          </Typography>
          <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <TabList onChange={(_, newValue) => handleTabChange(newValue)}>
              <Tab label="Overview" {...a11yProps('overview')} />
              <Tab label="Realms Permissions" {...a11yProps('permissions')} />
              <Tab label="Full Listing" {...a11yProps('listing')} />
            </TabList>
          </Box>
          <TabPanel value="overview" role="tabpanel" id="tabpanel-overview">
            <GroupForm name={name} refetchList={refetchList} />
          </TabPanel>
          <TabPanel
            value="permissions"
            role="tabpanel"
            id="tabpanel-permissions"
          >
            <span> WIP Permissions Tab</span>
          </TabPanel>
          <TabPanel value="listing" role="tabpanel" id="tabpanel-listing">
            <span> WIP Full Listing Tab</span>
          </TabPanel>
        </Box>
      </TabContext>
    </>
  );
}

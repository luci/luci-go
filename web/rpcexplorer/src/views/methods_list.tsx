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

import { Link as RouterLink, useParams } from 'react-router-dom';

import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';

import { Doc } from '../components/doc';
import { MethodIcon } from '../components/icons';
import { useGlobals } from '../context/globals';


const MethodsList = () => {
  const { serviceName } = useParams();
  const { descriptors } = useGlobals();

  const svc = descriptors.service(serviceName ?? 'unknown');
  if (svc === undefined) {
    return (
      <Alert severity='error'>
        Service <b>{serviceName ?? 'unknown'}</b> is not
        registered in the server.
      </Alert>
    );
  }

  return (
    <Stack>
      <Doc markdown={svc.doc} />
      <List dense>
        {svc.methods.map((method) => {
          return (
            <ListItem key={method.name} disablePadding divider>
              <ListItemButton component={RouterLink} to={method.name}>
                <ListItemIcon sx={{ minWidth: '40px' }}>
                  <MethodIcon />
                </ListItemIcon>
                <ListItemText primary={method.name} secondary={method.help} />
              </ListItemButton>
            </ListItem>
          );
        })}
      </List>
    </Stack>
  );
};


export default MethodsList;

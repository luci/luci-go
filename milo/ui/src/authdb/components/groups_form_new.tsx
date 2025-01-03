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
import { FormControl } from '@mui/material';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { createTheme, ThemeProvider } from '@mui/material/styles';
import { useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';

import {
  addPrefixToItems,
  isGlob,
  nameRe,
  isMember,
  isSubgroup,
} from '@/authdb/common/helpers';
import { useAuthServiceClient } from '@/authdb/hooks/prpc_clients';
import { getURLPathFromAuthGroup } from '@/common/tools/url_utils';
import {
  AuthGroup,
  CreateGroupRequest,
} from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

const theme = createTheme({
  typography: {
    h6: {
      color: 'black',
      margin: '0',
      padding: '0',
      fontSize: '1.17em',
      fontWeight: 'bold',
    },
    subtitle1: {
      color: 'red',
      fontStyle: 'italic',
    },
  },
  components: {
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderBottom: 'none',
          padding: '0',
        },
      },
    },
  },
});

interface GroupsFormNewProps {
  onCreate: () => void;
}

export function GroupsFormNew({ onCreate }: GroupsFormNewProps) {
  const navigate = useNavigate();
  const [name, setName] = useState<string>('');
  const [nameErrorMessage, setNameErrorMessage] = useState<string>('');
  const [description, setDescription] = useState<string>('');
  const [descriptionErrorMessage, setDescriptionErrorMessage] =
    useState<string>('');
  const [owners, setOwners] = useState<string>('');
  const [ownersErrorMessage, setOwnersErrorMessage] = useState<string>('');
  const [members, setMembers] = useState<string>('');
  const [membersErrorMessage, setMembersErrorMessage] = useState<string>('');
  const [globs, setGlobs] = useState<string>('');
  const [globsErrorMessage, setGlobsErrorMessage] = useState<string>('');
  const [subgroups, setSubgroups] = useState<string>('');
  const [subgroupsErrorMessage, setSubgroupsErrorMessage] =
    useState<string>('');
  const [errorMessage, setErrorMessage] = useState<string>();

  const client = useAuthServiceClient();
  const createMutation = useMutation({
    mutationFn: (request: CreateGroupRequest) => {
      return client.CreateGroup(request);
    },
    onSuccess: (response) => {
      setErrorMessage('');
      onCreate();
      navigate(getURLPathFromAuthGroup(response.name), { replace: true });
    },
    onError: () => {
      setErrorMessage('Error creating group');
    },
    onSettled: () => {},
  });

  const createGroup = () => {
    const validateFields = [
      validateName(),
      validateDescription(),
      validateOwners(),
      validateMembers(),
      validateGlobs(),
      validateSubgroups(),
    ];
    if (validateFields.every(Boolean)) {
      submitForm();
    }
  };

  const submitForm = () => {
    const membersArray = members.split(/[\n ]+/).filter((item) => item !== '');
    const globsArray = globs.split(/[\n ]+/).filter((item) => item !== '');
    const subgroupsArray = subgroups
      .split(/[\n ]+/)
      .filter((item) => item !== '');
    const membersArr = membersArray
      ? addPrefixToItems('user', membersArray)
      : [];
    const subgroupsArr = subgroupsArray || [];
    const globsArr = globsArray ? addPrefixToItems('user', globsArray) : [];
    const newGroup = AuthGroup.fromPartial({
      name: name,
      description: description,
      owners: owners,
      nested: subgroupsArr,
      members: membersArr,
      globs: globsArr,
    });
    createMutation.mutate({ group: newGroup });
  };

  const validateDescription = () => {
    let message = '';
    if (!description) {
      message = 'Description is required.';
    }
    setDescriptionErrorMessage(message);
    return message === '';
  };

  const validateOwners = () => {
    let message = '';
    if (owners && !nameRe.test(owners)) {
      message = 'Invalid owners name. Must be a group.';
    }
    setOwnersErrorMessage(message);
    return message === '';
  };

  const validateName = () => {
    let message = '';
    if (!nameRe.test(name)) {
      message = 'Invalid group name.';
    }
    setNameErrorMessage(message);
    return message === '';
  };

  const validateMembers = () => {
    let message = '';
    const membersArray = members.split(/[\n ]+/).filter((item) => item !== '');
    const invalidMembers = membersArray.filter((member) => !isMember(member));
    if (invalidMembers.length > 0) {
      message = 'Invalid members: ' + invalidMembers.join(', ');
    }
    setMembersErrorMessage(message);
    return message === '';
  };

  const validateGlobs = () => {
    let message = '';
    const globsArray = globs.split(/[\n ]+/).filter((item) => item !== '');
    const invalidGlobs = globsArray.filter((glob) => !isGlob(glob));
    if (invalidGlobs.length > 0) {
      message = 'Invalid globs: ' + invalidGlobs.join(', ');
    }
    setGlobsErrorMessage(message);
    return message === '';
  };

  const validateSubgroups = () => {
    let message = '';
    const subgroupsArray = subgroups
      .split(/[\n ]+/)
      .filter((item) => item !== '');
    const invalidSubgroups = subgroupsArray.filter(
      (subgroup) => !isSubgroup(subgroup),
    );
    if (invalidSubgroups.length > 0) {
      message = 'Invalid subgroups: ' + invalidSubgroups.join(', ');
    }
    setSubgroupsErrorMessage(message);
    return message === '';
  };

  return (
    <Box sx={{ minHeight: '500px', p: '20px' }}>
      <ThemeProvider theme={theme}>
        <FormControl data-testid="groups-form-new" style={{ width: '100%' }}>
          <Typography variant="h5"> Create new group </Typography>
          <TableContainer sx={{ p: 0, width: '100%' }}>
            <Table>
              <TableBody>
                <TableRow>
                  <TableCell
                    style={{
                      display: 'flex',
                      flexDirection: 'row',
                      alignItems: 'center',
                      minHeight: '45px',
                    }}
                  >
                    <Typography variant="h6"> Name</Typography>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    align="left"
                    style={{ width: '95%' }}
                    sx={{ pb: '8px' }}
                  >
                    <TextField
                      value={name}
                      style={{ width: '100%' }}
                      onChange={(e) => setName(e.target.value)}
                      onBlur={validateName}
                      id="nameTextfield"
                      data-testid="name-textfield"
                      placeholder="required"
                      error={nameErrorMessage !== ''}
                      helperText={nameErrorMessage}
                    ></TextField>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    style={{
                      display: 'flex',
                      flexDirection: 'row',
                      alignItems: 'center',
                      minHeight: '45px',
                    }}
                  >
                    <Typography variant="h6"> Description</Typography>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    align="left"
                    style={{ width: '95%' }}
                    sx={{ pb: '8px' }}
                  >
                    <TextField
                      value={description}
                      style={{ width: '100%', minHeight: '60px' }}
                      onChange={(e) => setDescription(e.target.value)}
                      onBlur={validateDescription}
                      id="descriptionTextfield"
                      data-testid="description-textfield"
                      placeholder="required"
                      error={descriptionErrorMessage !== ''}
                      helperText={descriptionErrorMessage}
                    ></TextField>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    style={{
                      display: 'flex',
                      flexDirection: 'row',
                      alignItems: 'center',
                      minHeight: '45px',
                    }}
                  >
                    <Typography variant="h6"> Owners</Typography>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    align="left"
                    style={{ width: '95%' }}
                    sx={{ pb: '8px' }}
                  >
                    <TextField
                      value={owners}
                      style={{ width: '100%', minHeight: '60px' }}
                      onChange={(e) => setOwners(e.target.value)}
                      onBlur={validateOwners}
                      id="ownersTextfield"
                      data-testid="owners-textfield"
                      placeholder="administrators"
                      error={ownersErrorMessage !== ''}
                      helperText={ownersErrorMessage}
                    ></TextField>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    style={{
                      display: 'flex',
                      flexDirection: 'row',
                      alignItems: 'center',
                      minHeight: '45px',
                    }}
                  >
                    <Typography variant="h6"> Members</Typography>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    align="left"
                    style={{ width: '95%' }}
                    sx={{ pb: '8px' }}
                  >
                    <TextField
                      placeholder="List of members, one per line (e.g., person@example.com, serviceAccount@project.com)"
                      label="List of members, one per line (e.g., person@example.com, serviceAccount@project.com)"
                      multiline
                      value={members}
                      style={{ width: '100%', minHeight: '60px' }}
                      onChange={(e) => setMembers(e.target.value)}
                      onBlur={validateMembers}
                      id="membersTextfield"
                      data-testid="members-textfield"
                      error={membersErrorMessage !== ''}
                      helperText={membersErrorMessage}
                    ></TextField>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    style={{
                      display: 'flex',
                      flexDirection: 'row',
                      alignItems: 'center',
                      minHeight: '45px',
                    }}
                  >
                    <Typography variant="h6"> Globs</Typography>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    align="left"
                    style={{ width: '95%' }}
                    sx={{ pb: '8px' }}
                  >
                    <TextField
                      placeholder="List of globs, one per line (e.g., *@google.com, project:project-prefix-*)"
                      label="List of globs, one per line (e.g., *@google.com, project:project-prefix-*)"
                      multiline
                      value={globs}
                      style={{ width: '100%', minHeight: '60px' }}
                      onChange={(e) => setGlobs(e.target.value)}
                      onBlur={validateGlobs}
                      id="globsTextfield"
                      data-testid="globs-textfield"
                      error={globsErrorMessage !== ''}
                      helperText={globsErrorMessage}
                    ></TextField>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    style={{
                      display: 'flex',
                      flexDirection: 'row',
                      alignItems: 'center',
                      minHeight: '45px',
                    }}
                  >
                    <Typography variant="h6"> Subgroups</Typography>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    align="left"
                    style={{ width: '95%' }}
                    sx={{ pb: '8px' }}
                  >
                    <TextField
                      placeholder="List of subgroups, one per line
                       (e.g., administrators, mdb/chrome-troopers, google/committers@chromium.org)"
                      label="List of subgroups, one per line (e.g., administrators, mdb/chrome-troopers, google/committers@chromium.org)"
                      multiline
                      value={subgroups}
                      style={{ width: '100%', minHeight: '60px' }}
                      onChange={(e) => setSubgroups(e.target.value)}
                      onBlur={validateSubgroups}
                      id="subgroupsTextfield"
                      data-testid="subgroups-textfield"
                      error={subgroupsErrorMessage !== ''}
                      helperText={subgroupsErrorMessage}
                    ></TextField>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </TableContainer>
          <div style={{ display: 'flex', flexDirection: 'row-reverse' }}>
            <Button
              variant="contained"
              style={{ width: '150px' }}
              sx={{ mt: 1.5 }}
              onClick={createGroup}
              data-testid="create-button"
            >
              Create Group
            </Button>
          </div>
          <div style={{ padding: '5px' }}>
            {errorMessage && <Alert severity="error">{errorMessage}</Alert>}
          </div>
        </FormControl>
      </ThemeProvider>
    </Box>
  );
}

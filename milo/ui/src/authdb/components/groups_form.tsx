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
import { createTheme, ThemeProvider } from '@mui/material/styles';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import EditIcon from '@mui/icons-material/Edit';
import DoneIcon from '@mui/icons-material/Done';
import Table from '@mui/material/Table';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import { TextareaAutosize } from '@mui/base/TextareaAutosize';
import Typography from '@mui/material/Typography';
import { FormControl } from '@mui/material';
import { useAuthServiceClient } from '@/authdb/hooks/prpc_clients';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useState, useEffect, createRef } from 'react';
import { GroupsFormList, FormListElement } from '@/authdb/components/groups_form_list';
import { GroupsFormListReadonly } from '@/authdb/components/groups_form_list_readonly';
import { AuthGroup, UpdateGroupRequest, DeleteGroupRequest } from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

interface GroupsFormProps {
  name: string;
  onDelete?: () => void;
}
const theme = createTheme({
  typography: {
    h6: {
      color: 'black',
    },
  },
  components: {
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderBottom: 'none',
        }
      }
    },
  },
});

// Strips '<prefix>:' from a string if it starts with it.
function stripPrefix(prefix: string, str: string) {
  if (!str) {
    return '';
  }
  if (str.slice(0, prefix.length + 1) == prefix + ':') {
    return str.slice(prefix.length + 1, str.length);
  } else {
    return str;
  }
};

// Appends '<prefix>:' to a string if it doesn't have a prefix.
const addPrefix = (prefix: string, str: string) => {
  return str.indexOf(':') == -1 ? prefix + ':' + str : str;
};

const addPrefixToItems = (prefix: string, items: string[]) => {
  return items.map((item) => addPrefix(prefix, item));
};

// True if group name starts with '<something>/' prefix, where
// <something> is a non-empty string.
function isExternalGroupName(name: string) {
  return name.indexOf('/') > 0;
}

export function GroupsForm({ name, onDelete = () => { } }: GroupsFormProps) {
  const [descriptionMode, setDescriptionMode] = useState<boolean>();
  const [ownersMode, setOwnersMode] = useState<boolean>();
  const [description, setDescription] = useState<string>();
  const [owners, setOwners] = useState<string>();
  const [showOwnersEdit, setShowOwnersEdit] = useState<boolean>();
  const [showDescriptionEdit, setShowDescriptionEdit] = useState<boolean>();
  const [isExternal, setIsExternal] = useState<boolean>();
  const [successEditedGroup, setSuccessEditedGroup] = useState<boolean>();
  const [errorMessage, setErrorMessage] = useState<string>();
  const [disableSubmit, setDisableSubmit] = useState<boolean>();
  const [openDeleteDialog, setOpenDeleteDialog] = useState<boolean>();
  const membersRef = createRef<FormListElement>();
  const subgroupsRef = createRef<FormListElement>();
  const globsRef = createRef<FormListElement>();

  const updateMutation = useMutation({
    mutationFn: (request: UpdateGroupRequest) => {
      return client.UpdateGroup(request);
    },
    onSuccess: (response) => {
      setSuccessEditedGroup(true);
      const members: string[] = (response?.members)?.map((member => stripPrefix('user', member))) || [] as string[];
      membersRef.current?.changeItems(members);
      globsRef.current?.changeItems(response?.globs as string[]);
      subgroupsRef.current?.changeItems(response?.nested as string[]);
    },
    onError: () => {
      setErrorMessage('Error editing group');
    },
    onSettled: () => {
      setDisableSubmit(false);
    }
  })

  const deleteMutation = useMutation({
    mutationFn: (request: DeleteGroupRequest) => {
      return client.DeleteGroup(request);
    },
    onSuccess: () => {
      onDelete();
    },
    onError: () => {
      setErrorMessage('Error deleting group');
    }
  })

  const client = useAuthServiceClient();
  const {
    isLoading,
    isError,
    data: response,
    error
  } = useQuery({
    ...client.GetGroup.query({ "name": name }),
    onSuccess: (response) => {
      setReadonlyMode();
      setIsExternal(isExternalGroupName(response?.name!));
      },
  })

  const setReadonlyMode = () => {
    setDescriptionMode(false);
    setOwnersMode(false);
    setErrorMessage("");
    setSuccessEditedGroup(false);
  }
  const changeDescriptionMode = () => {
    setDescriptionMode(!descriptionMode);
  }
  useEffect(() => {
    if (descriptionMode) {
      addDescriptionEventListener();
    }
    if (ownersMode) {
      addOwnersEventListener();
    }
  }, [descriptionMode, ownersMode]);
  const changeOwnersMode = () => {
    setOwnersMode(!ownersMode);
  }

  const addDescriptionEventListener = () => {
    const descriptionTextfield = document.getElementById('descriptionTextfield');
    if (descriptionTextfield) {
      descriptionTextfield.addEventListener('keydown', (e) => {
        if (e.key == 'Enter') {
          setDescriptionMode(false);
        };
      });
    }
  }

  const addOwnersEventListener = () => {
    const ownersTextfield = document.getElementById('ownersTextfield');
    if (ownersTextfield) {
      ownersTextfield.addEventListener('keydown', (e) => {
        if (e.key == 'Enter') {
          setOwnersMode(false);
        }
      });
    }
  }

  const resetForm = () => {
    setDescriptionMode(false);
    setOwnersMode(false);
    membersRef.current?.setReadonly();
    globsRef.current?.setReadonly();
    subgroupsRef.current?.setReadonly();
  }

  const submitForm = () => {
    setDisableSubmit(true);
    resetForm();
    setReadonlyMode();
    const editedMembers = addPrefixToItems('user', membersRef.current?.getItems()!);
    const editedSubgroups = subgroupsRef.current?.getItems();
    const editedGlobs = addPrefixToItems('user', globsRef.current?.getItems()!);
    const editedGroup = AuthGroup.fromPartial({
      "name": name,
      "description": description || "",
      "owners": owners || "",
      "etag": etag || "",
      "nested": editedSubgroups || [],
      "members": editedMembers,
      "globs": editedGlobs,
    });
    updateMutation.mutate({ 'group': editedGroup, updateMask: undefined });
  }

  const deleteGroup = () => {
    setOpenDeleteDialog(false);
    deleteMutation.mutate({ 'name': name, 'etag': etag || '' });
  }

  const handleDeleteDialogClose = () => {
    setOpenDeleteDialog(false);
  }

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (isError) {
    return (
      <div className="section" data-testid="groups-form-error">
        <Alert severity="error">
          <AlertTitle>Failed to load groups form </AlertTitle>
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  // Set the state values outside of onSuccess.
  // This is necessary due to the queryKey implicitly using the auth state,
  // which changes if the user navigates away from the page.
  if (description == null) {
    setDescription(response?.description || '');
  }
  if (owners == null) {
    setOwners(response?.owners || '');
  }
  const members: string[] = (response?.members)?.map((member => stripPrefix('user', member))) || [] as string[];
  const subgroups: string[] = (response?.nested || []) as string[];
  const globs: string[] = (response?.globs || []) as string[];
  const etag = response?.etag;
  const callerCanModify: boolean = response?.callerCanModify || false;

  return (
    <Box sx={{ minHeight: '500px', p: '20px', ml: '5px' }}>
      <ThemeProvider theme={theme}>
        <FormControl data-testid="groups-form" style={{ width: '100%' }}>
          <Typography variant="h5" sx={{ pl: 1.5 }}> {name} </Typography>
          {!isExternal &&
            <TableContainer sx={{ p: 0, width: '100%' }} >
              <Table onMouseEnter={() => setShowDescriptionEdit(true)} onMouseLeave={() => setShowDescriptionEdit(false)}>
                <TableRow>
                  <TableCell sx={{ pb: 0 }} style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', minHeight: '45px' }}>
                    <Typography variant="h6"> Description</Typography>
                    {(showDescriptionEdit || descriptionMode) && callerCanModify &&
                      <IconButton color='primary' onClick={changeDescriptionMode} sx={{ p: 0, ml: 1.5 }}>
                        {descriptionMode
                          ? <DoneIcon />
                          : <EditIcon />
                        }
                      </IconButton>
                    }
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell align='left' style={{ width: '95%' }} sx={{ pt: 0 }}>
                    {descriptionMode
                      ? <TextareaAutosize value={description} style={{ width: '100%', whiteSpace: 'pre-wrap' }} onChange={(e) => setDescription(e.target.value)} id='descriptionTextfield'></TextareaAutosize>
                      : <Typography variant="body2" style={{ width: '100%' }}> {description} </Typography>
                    }
                  </TableCell>
                </TableRow>
              </Table>
              <Table onMouseEnter={() => setShowOwnersEdit(true)} onMouseLeave={() => setShowOwnersEdit(false)}>
                <TableRow >
                  <TableCell sx={{ pb: 0 }} style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', minHeight: '45px' }}>
                    <Typography variant="h6"> Owners</Typography>
                    {(showOwnersEdit || ownersMode) && callerCanModify &&
                      <IconButton color='primary' onClick={changeOwnersMode} sx={{ p: 0, ml: 1.5 }}>
                        {ownersMode
                          ? <DoneIcon />
                          : <EditIcon />
                        }
                      </IconButton>
                    }
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell align='left' style={{ width: '95%' }} sx={{ pt: 0 }}>
                    {ownersMode
                      ? <TextareaAutosize value={owners} style={{ width: '100%' }} onChange={(e) => setOwners(e.target.value)} id='ownersTextfield'></TextareaAutosize>
                      : <Typography variant="body2" style={{ width: '100%' }}> {owners} </Typography>
                    }
                  </TableCell>
                </TableRow>
              </Table>
            </TableContainer>}
          {isExternal
            ?
            <GroupsFormListReadonly name='Members' initialItems={members} />
            :
            <>
              {callerCanModify ?
                <>
                  <GroupsFormList name='Members' initialItems={members} ref={membersRef} />
                  <GroupsFormList name='Globs' initialItems={globs} ref={globsRef} />
                  <GroupsFormList name='Subgroups' initialItems={subgroups} ref={subgroupsRef} />
                  <div style={{ display: 'flex', flexDirection: 'row' }}>
                    <Button variant="contained" disableElevation style={{ width: '15%' }} sx={{ mt: 1.5, ml: 1.5 }} onClick={submitForm} data-testid='submit-button' disabled={disableSubmit}>
                      Update Group
                    </Button>
                    <Button variant="contained" color="error" disableElevation style={{ width: '15%' }} sx={{ mt: 1.5, ml: 1.5 }} onClick={() => setOpenDeleteDialog(true)} data-testid='delete-button'>
                      Delete Group
                    </Button>
                  </div>
                  <div style={{ padding: '5px' }}>
                    {successEditedGroup &&
                      <Alert severity="success">Group updated</Alert>
                    }
                    {errorMessage &&
                      <Alert severity="error">{errorMessage}</Alert>
                    }
                  </div>
                </>
                :
                <>
                  <GroupsFormListReadonly name='Members' initialItems={members} />
                  <GroupsFormListReadonly name='Globs' initialItems={globs} />
                  <GroupsFormListReadonly name='Subgroups' initialItems={subgroups} />
                  <Typography variant="caption" sx={{ p: '16px' }} style={{ fontStyle: 'italic' }}>You do not have sufficient permissions to modify this group.</Typography>
                </>
              }
            </>
          }
        </FormControl>
        <Dialog
          open={openDeleteDialog || false}
          onClose={handleDeleteDialogClose}
          data-testid="delete-confirm-dialog"
        >
          <DialogTitle>
            {`Are you sure you want to delete this group: ${name}?`}
          </DialogTitle>
          <DialogActions>
            <Button onClick={handleDeleteDialogClose} disableElevation variant="outlined">Cancel</Button>
            <Button onClick={deleteGroup} disableElevation variant="contained" color="error" data-testid='delete-confirm-button'>Delete</Button>
          </DialogActions>
        </Dialog>
      </ThemeProvider>
    </Box>
  );
}

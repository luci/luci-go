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

import { GrpcError } from '@chopsui/prpc-client';
import DoneIcon from '@mui/icons-material/Done';
import EditIcon from '@mui/icons-material/Edit';
import { FormControl } from '@mui/material';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import CircularProgress from '@mui/material/CircularProgress';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useState, createRef, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router';

import { nameRe, stripPrefix } from '@/authdb/common/helpers';
import { AuthTableList } from '@/authdb/components/auth_table_list';
import { GroupLink } from '@/authdb/components/group_link';
import {
  GroupsFormList,
  FormListElement,
} from '@/authdb/components/groups_form_list';
import { useAuthServiceGroupsClient } from '@/authdb/hooks/prpc_clients';
import { getAuthGroupURLPath } from '@/common/tools/url_utils';
import {
  AuthGroup,
  UpdateGroupRequest,
  DeleteGroupRequest,
} from '@/proto/go.chromium.org/luci/auth_service/api/rpcpb/groups.pb';

// True if group name starts with '<something>/' prefix, where
// <something> is a non-empty string.
function isExternalGroupName(name: string) {
  return name.indexOf('/') > 0;
}

interface GroupFormProps {
  name: string;
  refetchList: (fresh: boolean) => void;
}

export function GroupForm({ name, refetchList }: GroupFormProps) {
  const navigate = useNavigate();
  const [descriptionMode, setDescriptionMode] = useState<boolean>(false);
  const [ownersMode, setOwnersMode] = useState<boolean>(false);
  const [description, setDescription] = useState<string>();
  const [owners, setOwners] = useState<string>();
  const [etag, setEtag] = useState<string>();
  const [showOwnersEdit, setShowOwnersEdit] = useState<boolean>();
  const [showDescriptionEdit, setShowDescriptionEdit] = useState<boolean>();
  const [isExternal, setIsExternal] = useState<boolean>();
  const [successEditedGroup, setSuccessEditedGroup] = useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string>('');
  const [isUpdating, setIsUpdating] = useState<boolean>();
  const [openDeleteDialog, setOpenDeleteDialog] = useState<boolean>();
  const [savedDescription, setSavedDescription] = useState<string>('');
  const [savedOwners, setSavedOwners] = useState<string>('');
  const [descriptionErrorMessage, setDescriptionErrorMessage] =
    useState<string>('');
  const [ownersErrorMessage, setOwnersErrorMessage] = useState<string>('');
  const membersRef = createRef<FormListElement>();
  const subgroupsRef = createRef<FormListElement>();
  const globsRef = createRef<FormListElement>();
  const client = useAuthServiceGroupsClient();
  const {
    isPending,
    isError,
    data: response,
    error,
    refetch,
  } = useQuery({
    ...client.GetGroup.query({ name: name }),
    refetchOnWindowFocus: false,
  });

  const updateMutation = useMutation({
    mutationFn: (request: UpdateGroupRequest) => {
      return client.UpdateGroup(request);
    },
    onSuccess: (response) => {
      const members: string[] =
        response?.members?.map((member) => stripPrefix('user', member)) ||
        ([] as string[]);
      membersRef.current?.changeItems(members);
      globsRef.current?.changeItems(response?.globs as string[]);
      subgroupsRef.current?.changeItems(response?.nested as string[]);
      if (!descriptionMode) {
        setDescription(response?.description);
      }
      if (!ownersMode) {
        setOwners(response?.owners);
      }
      setSavedDescription(response?.description);
      setSavedOwners(response?.owners);
      setEtag(response?.etag);
      // Refetch to update values as cache will display previous values.
      refetch().then(() => {
        setIsUpdating(false);
        setSuccessEditedGroup(true);
        // Group updates don't strictly need the latest list of groups.
        refetchList(false);
      });
    },
    onError: (error: GrpcError) => {
      setErrorMessage(`Error editing group - ${error.description}`);
      setIsUpdating(false);
      // Reset all fields to saved values, otherwise an invalid value will be showing.
      membersRef.current?.resetToSavedValues();
      globsRef.current?.resetToSavedValues();
      subgroupsRef.current?.resetToSavedValues();
      // If there is an error editing, maintain the user's draft changes for description and owners.
      if (description !== savedDescription) {
        setDescriptionMode(true);
      }
      if (owners !== savedOwners) {
        setOwnersMode(true);
      }
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (request: DeleteGroupRequest) => {
      return client.DeleteGroup(request);
    },
    onSuccess: () => {
      navigate(getAuthGroupURLPath('administrators'), {
        replace: true,
      });
      // The list of groups must be the latest, to ensure the group that
      // was just deleted is removed.
      refetchList(true);
    },
    onError: (error: GrpcError) => {
      setErrorMessage(`Error deleting group - ${error.description}`);
    },
  });

  const changeDescriptionMode = () => {
    if (descriptionMode) {
      submitField('description');
    } else {
      setDescriptionMode(!descriptionMode);
    }
  };

  const changeOwnersMode = () => {
    if (ownersMode) {
      submitField('owners');
    } else {
      setOwnersMode(!ownersMode);
    }
  };

  const submitForm = (updateMask: string[]) => {
    setIsUpdating(true);
    setErrorMessage('');
    setSuccessEditedGroup(false);
    const editedMembers = membersRef.current?.getItems();
    const editedSubgroups = subgroupsRef.current?.getItems();
    const editedGlobs = globsRef.current?.getItems();
    const editedGroup = AuthGroup.fromPartial({
      name: name,
      description: description || '',
      owners: owners || '',
      etag: etag || '',
      nested: editedSubgroups || [],
      members: editedMembers,
      globs: editedGlobs,
    });

    updateMutation.mutate({ group: editedGroup, updateMask: updateMask });
  };

  const checkFieldSubmit = (keyPressed: string, field: string) => {
    if (keyPressed !== 'Enter') {
      return;
    }
    submitField(field);
  };

  const validateDescription = useCallback(() => {
    let message = '';
    if (!description) {
      message = 'Description is required.';
    }
    setDescriptionErrorMessage(message);
    return message === '';
  }, [description]);

  const validateOwners = useCallback(() => {
    let message = '';
    if (owners && !nameRe.test(owners)) {
      message = 'Invalid owners name. Must be a group.';
    }
    setOwnersErrorMessage(message);
    return message === '';
  }, [owners]);

  useEffect(() => {
    validateDescription();
  }, [description, validateDescription]);

  useEffect(() => {
    validateOwners();
  }, [owners, validateOwners]);

  const submitField = (field: string) => {
    if (field === 'description') {
      if (!validateDescription()) {
        return;
      }
      setDescriptionMode(false);
      if (description === savedDescription) {
        return;
      }
    } else if (field === 'owners') {
      if (!validateOwners()) {
        return;
      }
      setOwnersMode(false);
      if (owners === savedOwners) {
        return;
      }
    }
    submitForm([field]);
  };

  const deleteGroup = () => {
    setOpenDeleteDialog(false);
    deleteMutation.mutate({ name: name, etag: etag || '' });
  };

  const handleDeleteDialogClose = () => {
    setOpenDeleteDialog(false);
  };

  if (isPending) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (isError) {
    return (
      <div className="section" data-testid="group-form-error">
        <Alert severity="error">
          <AlertTitle>Failed to load groups form </AlertTitle>
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  const initialDescription = response?.description;
  if (description === undefined) {
    setSavedDescription(initialDescription || '');
    setDescription(initialDescription || '');
  }
  const initialOwners = response?.owners;
  if (owners === undefined) {
    setSavedOwners(initialOwners || '');
    setOwners(initialOwners || '');
  }
  if (etag === undefined) {
    setEtag(response?.etag);
  }

  const members: string[] = (response?.members || []) as string[];
  const subgroups: string[] = (response?.nested || []) as string[];
  const globs: string[] = (response?.globs || []) as string[];
  const callerCanModify: boolean = response?.callerCanModify || false;
  const callerCanViewMembers: boolean = response?.callerCanViewMembers || false;
  const numRedacted: number = response?.numRedacted || 0;
  if (isExternal === undefined) {
    setIsExternal(isExternalGroupName(response?.name || ''));
  }

  return (
    <>
      <FormControl data-testid="group-form" style={{ width: '100%' }}>
        {!isExternal && (
          <TableContainer sx={{ p: 0, width: '100%' }}>
            <Table data-testid="description-table">
              <TableBody>
                <TableRow
                  data-testid="description-row"
                  onMouseEnter={() => setShowDescriptionEdit(true)}
                  onMouseLeave={() => setShowDescriptionEdit(false)}
                >
                  <TableCell>
                    <Typography variant="h6">Description</Typography>
                    {(showDescriptionEdit || descriptionMode) &&
                      callerCanModify && (
                        <IconButton
                          color="primary"
                          onClick={changeDescriptionMode}
                          sx={{ p: 0, ml: 1.5 }}
                          data-testid="edit-description-icon"
                        >
                          {descriptionMode ? <DoneIcon /> : <EditIcon />}
                        </IconButton>
                      )}
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    align="left"
                    style={{ width: '95%' }}
                    sx={{ pb: '16px' }}
                  >
                    {descriptionMode ? (
                      <TextField
                        multiline
                        value={description}
                        style={{
                          width: '100%',
                          whiteSpace: 'pre-wrap',
                        }}
                        onChange={(e) => setDescription(e.target.value)}
                        id="descriptionTextfield"
                        data-testid="description-textfield"
                        error={descriptionErrorMessage !== ''}
                        helperText={descriptionErrorMessage}
                        onBlur={validateDescription}
                      ></TextField>
                    ) : (
                      <Typography
                        variant="body2"
                        style={{ width: '100%' }}
                        whiteSpace="pre-line"
                      >
                        {description}
                      </Typography>
                    )}
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
            <Table
              onMouseEnter={() => setShowOwnersEdit(true)}
              onMouseLeave={() => setShowOwnersEdit(false)}
              data-testid="owners-table"
            >
              <TableBody>
                <TableRow>
                  <TableCell>
                    <Typography variant="h6">Owners</Typography>
                    {(showOwnersEdit || ownersMode) && callerCanModify && (
                      <IconButton
                        color="primary"
                        onClick={changeOwnersMode}
                        sx={{ p: 0, ml: 1.5 }}
                        data-testid="edit-owners-icon"
                      >
                        {ownersMode ? <DoneIcon /> : <EditIcon />}
                      </IconButton>
                    )}
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell
                    align="left"
                    style={{ width: '95%' }}
                    sx={{ pb: '16px' }}
                  >
                    {ownersMode ? (
                      <TextField
                        value={owners}
                        style={{ width: '100%' }}
                        onChange={(e) => setOwners(e.target.value)}
                        onKeyDown={(e) => checkFieldSubmit(e.key, 'owners')}
                        id="ownersTextfield"
                        data-testid="owners-textfield"
                        error={ownersErrorMessage !== ''}
                        helperText={ownersErrorMessage}
                        onBlur={validateOwners}
                        placeholder="administrators"
                      ></TextField>
                    ) : (
                      owners && <GroupLink name={owners} />
                    )}
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </TableContainer>
        )}
        {isExternal ? (
          <>
            {callerCanViewMembers ? (
              <AuthTableList name="Members" items={members} />
            ) : (
              <Typography variant="h6" sx={{ p: 1.5 }}>
                {' '}
                {numRedacted} members redacted
              </Typography>
            )}
          </>
        ) : (
          <>
            {callerCanModify ? (
              <>
                {callerCanViewMembers ? (
                  <GroupsFormList
                    name="Members"
                    initialValues={members}
                    ref={membersRef}
                    submitValues={() => submitField('members')}
                  />
                ) : (
                  <Typography variant="h6" sx={{ p: '16px' }}>
                    {' '}
                    {numRedacted} members redacted
                  </Typography>
                )}
                <GroupsFormList
                  name="Globs"
                  initialValues={globs}
                  ref={globsRef}
                  submitValues={() => submitField('globs')}
                />
                <GroupsFormList
                  name="Subgroups"
                  initialValues={subgroups}
                  ref={subgroupsRef}
                  submitValues={() => submitField('nested')}
                />
                <div>
                  {successEditedGroup && (
                    <Alert severity="success" sx={{ mt: 1.5 }}>
                      Group updated
                    </Alert>
                  )}
                  {errorMessage && (
                    <Alert severity="error" sx={{ mt: 1.5 }}>
                      {errorMessage}
                    </Alert>
                  )}
                </div>
                {isUpdating && <CircularProgress></CircularProgress>}
                {name !== 'administrators' && (
                  <div
                    style={{
                      display: 'flex',
                      flexDirection: 'row-reverse',
                    }}
                  >
                    <Button
                      variant="outlined"
                      color="error"
                      disableElevation
                      style={{ width: '170px' }}
                      sx={{ mt: 1.5, ml: 1.5 }}
                      onClick={() => setOpenDeleteDialog(true)}
                      data-testid="delete-button"
                    >
                      Delete Group
                    </Button>
                  </div>
                )}
              </>
            ) : (
              <>
                {callerCanViewMembers ? (
                  <AuthTableList name="Members" items={members} />
                ) : (
                  <Typography variant="h6" sx={{ p: '16px' }}>
                    {' '}
                    {numRedacted} members redacted
                  </Typography>
                )}
                <AuthTableList name="Globs" items={globs} />
                <AuthTableList
                  name="Subgroups"
                  items={subgroups}
                  renderAsGroupLinks={true}
                />
                <Typography
                  variant="caption"
                  sx={{ p: '16px' }}
                  style={{ fontStyle: 'italic' }}
                >
                  You do not have sufficient permissions to modify this group.
                </Typography>
              </>
            )}
          </>
        )}
      </FormControl>
      <Dialog
        open={openDeleteDialog || false}
        onClose={handleDeleteDialogClose}
        data-testid="delete-confirm-dialog"
      >
        <DialogTitle>
          {`Are you sure you want to delete this group: ${name}? This action is irreversible.`}
        </DialogTitle>
        <DialogActions>
          <Button
            onClick={handleDeleteDialogClose}
            disableElevation
            variant="outlined"
          >
            Cancel
          </Button>
          <Button
            onClick={deleteGroup}
            disableElevation
            variant="contained"
            color="error"
            data-testid="delete-confirm-button"
          >
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}

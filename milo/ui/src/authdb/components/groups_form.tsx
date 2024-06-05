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
import IconButton from '@mui/material/IconButton';
import EditIcon from '@mui/icons-material/Edit';
import ClearIcon from '@mui/icons-material/Clear';
import CancelIcon from '@mui/icons-material/Cancel';
import AddCircleIcon from '@mui/icons-material/AddCircle';
import DoneIcon from '@mui/icons-material/Done';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { FormControl } from '@mui/material';
import { useAuthServiceClient } from '@/authdb/hooks/prpc_clients';
import { useQuery } from '@tanstack/react-query';
import { useState, useEffect } from 'react';

interface GroupsFormProps {
    name: string;
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
function stripPrefix (prefix: string, str: string) {
    if (!str) {
      return '';
    }
    if (str.slice(0, prefix.length + 1) == prefix + ':') {
      return str.slice(prefix.length + 1, str.length);
    } else {
      return str;
    }
};

export function GroupsForm({ name } : GroupsFormProps) {
  const [descriptionMode, setDescriptionMode] = useState<boolean>();
  const [ownersMode, setOwnersMode] = useState<boolean>();
  const [membersMode, setMembersMode] = useState<boolean>();
  const [subgroupsMode, setSubgroupsMode] = useState<boolean>();
  const [addingMember, setAddingMember] = useState<boolean>();
  const [addingSubgroup, setAddingSubgroup] = useState<boolean>();
  const [members, setMembers] = useState<string[]>();
  const [subgroups, setSubgroups] = useState<string[]>();
  const [description, setDescription] = useState<string>();
  const [owners, setOwners] = useState<string>();
  const [currentMember, setCurrentMember] = useState<string>();
  const [currentSubgroup, setCurrentSubgroup] = useState<string>();

  const client = useAuthServiceClient();
    const {
      isLoading,
      isError,
      error
    } = useQuery({
      ...client.GetGroup.query({"name": name}),
      onSuccess: (response) => {
          let initialMembers: string[] = (response?.members)?.map((member => stripPrefix('user', member))) || [] as string[];
          setMembers(initialMembers);
          let initialSubgroups: string[] = (response?.nested || []) as string[];
          setSubgroups(initialSubgroups);
          let initialDescription: string = response?.description || "";
          setDescription(initialDescription);
          let initialOwners: string = response?.owners || "";
          setOwners(initialOwners);
          setReadonlyMode();
      },
    })

    const setReadonlyMode = () => {
      setDescriptionMode(false);
      setOwnersMode(false);
      setMembersMode(false);
      setSubgroupsMode(false);
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
    const changeMembersMode = () => {
      setMembersMode(!membersMode);
    }
    const changeSubgroupsMode = () => {
      setSubgroupsMode(!subgroupsMode);
    }
    const changeAddingMember = () => {
      setAddingMember(!addingMember);
    }
    const changeAddingSubgroup = () => {
      setAddingSubgroup(!addingSubgroup);
    }

    const membersTextfield = document.getElementById('membersTextfield');
    if (membersTextfield) {
      membersTextfield.addEventListener('keydown', (e) => {
        if (e.key == 'Enter') {
          addToMembers(currentMember);
        }
      });
    }

    const subgroupsTextfield = document.getElementById('subgroupsTextfield');
    if (subgroupsTextfield) {
      subgroupsTextfield.addEventListener('keydown', (e) => {
        if (e.key == 'Enter') {
          addToSubgroups(currentSubgroup);
        }
      });
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

    const addToMembers = (name: string | undefined) => {
      if (name) {
        let newMembers = Array.from(members || []);
        newMembers.push(name);
        setMembers(newMembers);
        setCurrentMember('');
      }
    }

    const addToSubgroups = (name: string | undefined) => {
      if (name) {
        let newSubgroups = Array.from(subgroups || []);
        newSubgroups.push(name);
        setSubgroups(newSubgroups);
        setCurrentSubgroup('');
      }
    }

    const removeFromMembers = (index: number) => {
      let newMembers = Array.from(members || []);
      newMembers.splice(index, 1);
      setMembers(newMembers);
    }

    const removeFromSubgroups = (index: number) => {
      let newSubgroups = Array.from(subgroups || []);
      newSubgroups.splice(index, 1);
      setSubgroups(newSubgroups);
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

  return (
    <Paper elevation={3} sx={{minHeight:'500px', p:'20px', ml:'5px'}}>
      <ThemeProvider theme={theme}>
      <FormControl data-testid="groups-form" style={{width:'100%'}}>
        <div>
        <Typography variant="h4"> {name} </Typography>
        </div>
        <TableContainer>
          <Table sx={{ p: 0, width: '100%' }}>
              <TableRow>
                <TableCell>
                  <Typography variant="h6"> Description</Typography>
                </TableCell>
                <TableCell>
                  {descriptionMode
                  ? <TextField value={description} style={{width: '100%', whiteSpace:'pre-wrap'}} onChange={(e) => setDescription(e.target.value)} id='descriptionTextfield'></TextField>
                  : <Typography variant="body1"> {description} </Typography>
                  }
                </TableCell>
                <TableCell align='right'>
                {descriptionMode
                    ? <IconButton color='success' onClick={changeDescriptionMode} sx={{p: 0}}>
                        <DoneIcon />
                      </IconButton>
                    : <IconButton color='primary' onClick={changeDescriptionMode} sx={{p: 0}}>
                        <EditIcon />
                      </IconButton>
                    }
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell>
                  <Typography variant="h6"> Owners</Typography>
                </TableCell>
                <TableCell>
                  {ownersMode
                  ? <TextField value={owners} style={{width: '100%'}} onChange={(e) => setOwners(e.target.value)} id='ownersTextfield'></TextField>
                  : <Typography variant="body1"> {owners} </Typography>
                  }
                </TableCell>
                <TableCell align='right'>
                  <IconButton color='primary' onClick={changeOwnersMode} sx={{p: 0}}>
                    {ownersMode
                    ? <DoneIcon />
                    : <EditIcon />
                    }
                  </IconButton>
                </TableCell>
              </TableRow>
          </Table>
        </TableContainer>
        <TableContainer>
          <Table sx={{ p: 0, pt: '15px', width: '100%' }}>
            <TableRow>
              <TableCell colSpan={2}>
                <Typography variant="h6"> Members</Typography>
              </TableCell>
              <TableCell align='right'>
                  <IconButton color='primary' onClick={changeMembersMode} sx={{p: 0}}>
                    {membersMode
                    ? <ClearIcon />
                    : <EditIcon />
                    }
                  </IconButton>
                </TableCell>
            </TableRow>
            {members && members.map((member, index) =>
              <TableRow key={index} style={{height: '34px'}} sx={{pb: 0, pt: 0, borderBottom: '1px solid grey'}}>
                <TableCell sx={{pb: 0, pt: 0}} colSpan={2}>{member}</TableCell>
                {membersMode && <TableCell align='right' sx={{pb: 0, pt: 0}}>
                  <IconButton color='error' sx={{p: 0}} onClick={() => removeFromMembers(index as number)}><CancelIcon />
                  </IconButton>
                </TableCell>}
              </TableRow>
            )}
            {membersMode && addingMember && (
              <TableRow>
                <TableCell sx={{p: 0, pt: '15px', pr: '15px'}} style={{width: '94%'}}>
                  <TextField label='New Member' style={{width: '100%'}} onChange={(e) => setCurrentMember(e.target.value)} id='membersTextfield' value={currentMember}></TextField>
                </TableCell>
                <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
                  <IconButton color='success' sx={{p: 0}} onClick={() => {addToMembers(currentMember)}}>
                    <DoneIcon />
                  </IconButton>
                </TableCell>
                <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
                  <IconButton color='error' sx={{p: 0}} onClick={changeAddingMember}>
                    <ClearIcon />
                  </IconButton>
                </TableCell>
              </TableRow>)
            }
            {membersMode && !addingMember &&
              <TableRow>
                <Button sx={{mt: '10px'}} variant="outlined" startIcon={<AddCircleIcon />} onClick={changeAddingMember}>
                  Add
                </Button>
            </TableRow>
            }
          </Table>
        </TableContainer>
        <TableContainer>
          <Table sx={{ p: 0, pt: '15px', width: '100%' }}>
            <TableRow>
              <TableCell colSpan={2}>
                <Typography variant="h6"> Subgroups</Typography>
              </TableCell>
              <TableCell align='right'>
                  <IconButton color='primary' onClick={changeSubgroupsMode} sx={{p: 0}}>
                    {subgroupsMode
                    ? <ClearIcon />
                    : <EditIcon />
                    }
                  </IconButton>
                </TableCell>
            </TableRow>
            {subgroups && subgroups.map((subgroup, index) =>
              <TableRow key={index} style={{height: '34px'}} sx={{pb: 0, pt: 0, borderBottom: '1px solid grey'}}>
                <TableCell sx={{pb: 0, pt: 0}} colSpan={2}>{subgroup}</TableCell>
                {subgroupsMode && <TableCell align='right' sx={{pb: 0, pt: 0}}>
                  <IconButton color='error' sx={{p: 0}} onClick={() => removeFromSubgroups(index as number)}><CancelIcon />
                  </IconButton>
                </TableCell>}
              </TableRow>
            )}
            {subgroupsMode && addingSubgroup && (
              <TableRow>
                <TableCell sx={{p: 0, pt: '15px', pr: '15px'}} style={{width: '94%'}}>
                  <TextField label='New Subgroup' style={{width: '100%'}} onChange={(e) => setCurrentSubgroup(e.target.value)} id='subgroupsTextfield' value={currentSubgroup}></TextField>
                </TableCell>
                <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
                  <IconButton color='success' sx={{p: 0}} onClick={() => {addToSubgroups(currentSubgroup)}}>
                    <DoneIcon />
                  </IconButton>
                </TableCell>
                <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
                  <IconButton color='error' sx={{p: 0}} onClick={changeAddingSubgroup}>
                    <ClearIcon />
                  </IconButton>
                </TableCell>
              </TableRow>)
            }
            {subgroupsMode && !addingSubgroup &&
              <TableRow>
                <Button sx={{mt: '10px'}} variant="outlined" startIcon={<AddCircleIcon />} onClick={changeAddingSubgroup}>
                  Add
                </Button>
            </TableRow>
            }
          </Table>
        </TableContainer>
        {(descriptionMode || ownersMode || membersMode || subgroupsMode) &&
          <Button variant="contained" disableElevation style={{width: '15%', marginTop: '15px'}}>
            Submit
          </Button>
        }
      </FormControl>
      </ThemeProvider>
    </Paper>
  );
}

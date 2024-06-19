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
import DoneIcon from '@mui/icons-material/Done';
import Table from '@mui/material/Table';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import { TextareaAutosize } from '@mui/base/TextareaAutosize';
import Typography from '@mui/material/Typography';
import { FormControl } from '@mui/material';
import { useAuthServiceClient } from '@/authdb/hooks/prpc_clients';
import { useQuery } from '@tanstack/react-query';
import { useState, useEffect } from 'react';
import { GroupsFormList } from '@/authdb/components/groups_form_list';

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
  const [description, setDescription] = useState<string>();
  const [owners, setOwners] = useState<string>();
  const [showOwnersEdit, setShowOwnersEdit] = useState<boolean>();
  const [showDescriptionEdit, setShowDescriptionEdit] = useState<boolean>();

  const client = useAuthServiceClient();
    const {
      isLoading,
      isError,
      data: response,
      error
    } = useQuery({
      ...client.GetGroup.query({"name": name}),
      onSuccess: (response) => {
          let initialDescription: string = response?.description || "";
          setDescription(initialDescription);
          let initialOwners: string = response?.owners || "";
          setOwners(initialOwners);
          setReadonlyMode();
      },
    })
    const members: string[] = (response?.members)?.map((member => stripPrefix('user', member))) || [] as string[];
    const subgroups: string[] = (response?.nested || []) as string[];
    const globs: string[] = (response?.globs || []) as string[];

    const setReadonlyMode = () => {
      setDescriptionMode(false);
      setOwnersMode(false);
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
    <Box sx={{minHeight:'500px', p:'20px', ml:'5px'}}>
      <ThemeProvider theme={theme}>
      <FormControl data-testid="groups-form" style={{width:'100%'}}>
        <Typography variant="h5" sx={{pl: 1.5}}> {name} </Typography>
        <TableContainer sx={{ p: 0, width: '100%' }} >
          <Table onMouseEnter={() => setShowDescriptionEdit(true)} onMouseLeave={() => setShowDescriptionEdit(false)}>
            <TableRow>
              <TableCell sx={{pb:0}} style={{display: 'flex', flexDirection: 'row', alignItems:'center', minHeight: '45px'}}>
                <Typography variant="h6"> Description</Typography>
                {(showDescriptionEdit || descriptionMode) &&
                <IconButton color='primary' onClick={changeDescriptionMode} sx={{p: 0, ml: 1.5}}>
                  {descriptionMode
                    ? <DoneIcon />
                    : <EditIcon />
                  }
                </IconButton>
                }
              </TableCell>
            </TableRow>
            <TableRow>
              <TableCell align='left' style={{width: '95%'}} sx={{pt: 0}}>
                {descriptionMode
                  ? <TextareaAutosize value={description} style={{width: '100%', whiteSpace:'pre-wrap'}} onChange={(e) => setDescription(e.target.value)} id='descriptionTextfield'></TextareaAutosize>
                  : <Typography variant="body2" style={{width: '100%'}}> {description} </Typography>
                  }
              </TableCell>
            </TableRow>
          </Table>
          <Table onMouseEnter={() => setShowOwnersEdit(true)} onMouseLeave={() => setShowOwnersEdit(false)}>
            <TableRow >
              <TableCell sx={{pb: 0}} style={{display: 'flex', flexDirection: 'row', alignItems:'center', minHeight: '45px'}}>
                <Typography variant="h6"> Owners</Typography>
                {(showOwnersEdit || ownersMode) &&
                <IconButton color='primary' onClick={changeOwnersMode} sx={{p: 0, ml: 1.5}}>
                  {ownersMode
                    ? <DoneIcon />
                    : <EditIcon />
                  }
                  </IconButton>
                }
              </TableCell>
            </TableRow>
            <TableRow>
              <TableCell align='left' style={{width: '95%'}} sx={{pt: 0}}>
              {ownersMode
                ? <TextareaAutosize value={owners} style={{width: '100%'}} onChange={(e) => setOwners(e.target.value)} id='ownersTextfield'></TextareaAutosize>
                : <Typography variant="body2" style={{width: '100%'}}> {owners} </Typography>
              }
              </TableCell>
            </TableRow>
          </Table>
        </TableContainer>
          <GroupsFormList name='Members' initialItems={members}/>
          <GroupsFormList name='Globs' initialItems={globs}/>
          <GroupsFormList name='Subgroups' initialItems={subgroups}/>
          <Button variant="contained" disableElevation style={{width: '15%'}} sx={{mt: 1.5, ml: 1.5}}>
            Submit
          </Button>
      </FormControl>
      </ThemeProvider>
    </Box>
  );
}

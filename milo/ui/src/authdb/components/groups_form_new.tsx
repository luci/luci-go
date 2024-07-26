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
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import { TextareaAutosize } from '@mui/base/TextareaAutosize';
import Typography from '@mui/material/Typography';
import { FormControl } from '@mui/material';
import { useState } from 'react';

const nameRe = /^([a-z\-]+\/)?[0-9a-z_\-\.@]{1,100}$/;

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

export function GroupsFormNew () {
  const [name, setName] = useState<string>('');
  const [nameErrorMessage, setNameErrorMessage] = useState<string>();
  const [description, setDescription] = useState<string>('');
  const [descriptionErrorMessage, setDescriptionErrorMessage] = useState<string>();
  const [owners, setOwners] = useState<string>('');
  const [ownersErrorMessage, setOwnersErrorMessage] = useState<string>();

    const createGroup = () => {
      if (!nameRe.test(name)) {
        setNameErrorMessage('Invalid group name.');
      } else {
        setNameErrorMessage('');
      }
      if (!description) {
        setDescriptionErrorMessage('Description is required.');
      } else {
        setDescriptionErrorMessage('');
      }
      if (!nameRe.test(owners)) {
        setOwnersErrorMessage('Invalid owners name. Must be a group.');
      } else {
        setOwnersErrorMessage('');
      }
    }

  return (
    <Box sx={{minHeight:'500px', p:'20px', ml:'5px'}}>
      <ThemeProvider theme={theme}>
      <FormControl data-testid="groups-form-new" style={{width:'100%'}}>
        <Typography variant="h5" sx={{pl: 1.5}}> Creating New Group </Typography>
        <TableContainer sx={{ p: 0, width: '100%' }} >
          <Table>
            <TableBody>
              <TableRow>
                <TableCell sx={{pb: 0}} style={{display: 'flex', flexDirection: 'row', alignItems:'center', minHeight: '45px'}}>
                  <Typography variant="h6"> Name</Typography>
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell align='left' style={{width: '95%'}} sx={{pt: 0, pb: '8px'}}>
                  <TextField value={name} style={{width: '100%'}} onChange={(e) => setName(e.target.value)} id='nameTextfield' data-testid='name-textarea' placeholder='required'></TextField>
                </TableCell>
              </TableRow>
              {nameErrorMessage &&
              <TableRow sx={{pt: 0}}>
                <TableCell sx={{pt: 0}}>
                  <Alert severity="error" data-testid='name-error'>{nameErrorMessage}</Alert>
                </TableCell>
              </TableRow>
              }
            </TableBody>
          </Table>
          <Table>
            <TableBody>
              <TableRow>
                <TableCell sx={{pb: 0}} style={{display: 'flex', flexDirection: 'row', alignItems:'center', minHeight: '45px'}}>
                  <Typography variant="h6"> Description</Typography>
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell align='left' style={{width: '95%'}} sx={{pt: 0, pb: '8px'}}>
                  <TextareaAutosize value={description} style={{width: '100%', minHeight: '60px'}} onChange={(e) => setDescription(e.target.value)} id='descriptionTextfield' data-testid='description-textarea' placeholder='required'></TextareaAutosize>
                </TableCell>
              </TableRow>
              {descriptionErrorMessage &&
              <TableRow sx={{pt: 0}}>
                <TableCell sx={{pt: 0}}>
                  <Alert severity="error" data-testid='description-error'>{descriptionErrorMessage}</Alert>
                </TableCell>
              </TableRow>
              }
            </TableBody>
          </Table>
          <Table>
            <TableBody>
              <TableRow>
                <TableCell sx={{pb: 0}} style={{display: 'flex', flexDirection: 'row', alignItems:'center', minHeight: '45px'}}>
                  <Typography variant="h6"> Owners</Typography>
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell align='left' style={{width: '95%'}} sx={{pt: 0, pb: '8px'}}>
                  <TextField value={owners} style={{width: '100%', minHeight: '60px'}} onChange={(e) => setOwners(e.target.value)} id='ownersTextfield' data-testid='owners-textfield' placeholder='administrators'></TextField>
                </TableCell>
              </TableRow>
              {ownersErrorMessage &&
              <TableRow sx={{pt: 0}}>
                <TableCell sx={{pt: 0}}>
                  <Alert severity="error" data-testid='owners-error'>{ownersErrorMessage}</Alert>
                </TableCell>
              </TableRow>
              }
            </TableBody>
          </Table>
        </TableContainer>
        <Button variant="contained" disableElevation style={{width: '150px'}} sx={{mt: 1.5, ml: '16px'}} onClick={createGroup} data-testid='create-button'>
            Create Group
        </Button>
      </FormControl>
      </ThemeProvider>
    </Box>
  );
}
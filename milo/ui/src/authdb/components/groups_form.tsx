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
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import List from '@mui/material/List';
import ListItemText from '@mui/material/ListItemText';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { FormControl } from '@mui/material';
import { FormLabel } from '@mui/material';
import { useAuthServiceClient } from '@/authdb/hooks/prpc_clients';
import { useQuery } from '@tanstack/react-query';

interface GroupsFormProps {
    name: string;
}

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
    const client = useAuthServiceClient();
    const {
      isLoading,
      isError,
      data: response,
      error,
    } = useQuery({
      ...client.GetGroup.query({"name": name})
    })
    const members: readonly string[] = (response?.members)?.map((member => stripPrefix('user', member))) || [];
    const description: string = response?.description || "";
    const owners: string = response?.owners || "";
    const subgroups: readonly string[] = response?.nested || [];

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
      <FormControl data-testid="groups-form">
        <Typography variant="h4" sx={{pb: 1.5}}> {name} </Typography>
        <TableContainer>
          <Table sx={{ p: 0, width: '100%' }}>
              <TableRow>
                <TableCell>
                  <Typography variant="h6"> Description</Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="body1"> {description} </Typography>
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell>
                  <Typography variant="h6"> Owners</Typography>
                </TableCell>
                <TableCell>
                  <Typography variant="body1"> {owners} </Typography>
                </TableCell>
              </TableRow>
          </Table>
        </TableContainer>
        <div>
          <FormLabel>
            <Typography variant="h6"> Members</Typography>
          </FormLabel>
          <List disablePadding>
            {members.map((member, index) => <ListItemText key={index}>{member}</ListItemText>)}
          </List>
        </div>
        <div>
          <FormLabel>
            <Typography variant="h6"> Subgroups</Typography>
          </FormLabel>
          <List disablePadding>
            {subgroups.map((subgroup, index) => <ListItemText key={index}>{subgroup}</ListItemText>)}
          </List>
        </div>
      </FormControl>
    </Paper>
  );
}

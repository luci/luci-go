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
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import EditIcon from '@mui/icons-material/Edit';
import ClearIcon from '@mui/icons-material/Clear';
import CancelIcon from '@mui/icons-material/Cancel';
import AddCircleIcon from '@mui/icons-material/AddCircle';
import DoneIcon from '@mui/icons-material/Done';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import {useState, useEffect} from 'react';

interface GroupsFormListProps {
    initialItems: string[];
    name: string;
}

export function GroupsFormList({ initialItems, name } :GroupsFormListProps) {
    const [editMode, setEditMode] = useState<boolean>();
    const [items, setItems] = useState<string[]>(initialItems);
    const [addingItem, setAddingItem] = useState<boolean>();
    const [currentItem, setCurrentItem] = useState<string>();
    const changeEditMode = () => {
        setEditMode(!editMode);
      }

    const changeAddingItem = () => {
        setAddingItem(!addingItem);
      }

    const addToItems = () => {
        if (currentItem) {
          setItems([...items, currentItem]);
          setCurrentItem('');
        }
    }

    const removeFromItems = (index: number) => {
        setItems(items.filter((_, i) => i != index));
    }

    const submitItem = (e: React.KeyboardEvent<HTMLDivElement>) => {
        if (e.key == 'Enter') {
          addToItems();
        }
      }

    useEffect(() => {
        setItems(initialItems)
    }, [initialItems])

  return (
    <TableContainer data-testid='groups-form-list'>
    <Table sx={{ p: 0, pt: '15px', width: '100%' }}>
        <TableBody>
      <TableRow>
        <TableCell colSpan={2}>
          <Typography variant="h6"> {name}</Typography>
        </TableCell>
        <TableCell align='right'>
            <IconButton color='primary' onClick={changeEditMode} sx={{p: 0}} data-testid='edit-button'>
              {editMode
              ? <ClearIcon />
              : <EditIcon />
              }
            </IconButton>
          </TableCell>
      </TableRow>
      {items && items.map((item, index) =>
        <TableRow key={index} style={{height: '34px'}} sx={{pb: 0, pt: 0, borderBottom: '1px solid grey'}}>
          <TableCell sx={{pb: 0, pt: 0}} colSpan={2}>{item}</TableCell>
          {editMode && <TableCell align='right' sx={{pb: 0, pt: 0}}>
            <IconButton color='error' sx={{p: 0}} onClick={() => removeFromItems(index as number)} data-testid={`remove-button-${item}`}><CancelIcon />
            </IconButton>
          </TableCell>}
        </TableRow>
      )}
      {editMode && addingItem && (
        <TableRow>
          <TableCell sx={{p: 0, pt: '15px', pr: '15px'}} style={{width: '94%'}}>
            <TextField label='Add New' style={{width: '100%'}} onChange={(e) => setCurrentItem(e.target.value)} onKeyDown={submitItem} value={currentItem} data-testid='add-textfield'></TextField>
          </TableCell>
          <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
            <IconButton color='success' sx={{p: 0}} onClick={() => {addToItems()}} data-testid='confirm-button'>
              <DoneIcon />
            </IconButton>
          </TableCell>
          <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
            <IconButton color='error' sx={{p: 0}} onClick={changeAddingItem}>
              <ClearIcon />
            </IconButton>
          </TableCell>
        </TableRow>)
      }
      {editMode && !addingItem &&
        <TableRow>
            <TableCell>
          <Button sx={{mt: '10px'}} variant="outlined" startIcon={<AddCircleIcon />} onClick={changeAddingItem} data-testid='add-button'>
            Add
          </Button>
          </TableCell>
      </TableRow>
      }
      </TableBody>
    </Table>
  </TableContainer>);
}

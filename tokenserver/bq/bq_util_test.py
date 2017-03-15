#!/usr/bin/env python

"""Unit tests for bq_util.py."""

import os
import pprint
import shutil
import tempfile
import unittest

import bq_util


PROJECT_ID = 'fake-project'
DATASET_ID = 'fake-dataset'


class BigQueryMock(object):
  def __init__(self, tables):
    self.tables = tables
    self.create_table_calls = []
    self.update_table_calls = []

  def list_datasets(self):
    return [DATASET_ID]

  def list_tables(self, dataset_id):
    assert dataset_id == DATASET_ID
    return self.tables.keys()

  def get_table_metadata(self, dataset_id, table_id):
    assert dataset_id == DATASET_ID
    assert table_id in self.tables
    return self.tables[table_id].copy()

  def create_table(self, **kwargs):
    self.create_table_calls.append(kwargs)

  def update_table(self, **kwargs):
    self.update_table_calls.append(kwargs)


class ArgsMock(object):
  def __init__(self, tmp_dir, schemas):
    self.project_id = PROJECT_ID
    self.dataset_id = DATASET_ID
    paths = []
    for schema in schemas:
      p = os.path.join(tmp_dir, schema['table_id'] + '.schema')
      with open(p, 'wt') as f:
        pprint.pprint(schema, stream=f)
      paths.append(p)
    setattr(self, 'schema-file', paths)


class TableFieldsTest(unittest.TestCase):
  def test_eq(self):
    def mk(*fields):
      return bq_util.TableFields(fields)

    def f(name, desc='', fields=None):
      return bq_util.TableFields.Field(name, 'INT', 'NULLABLE', desc, fields)

    self.assertEqual(mk(), mk())
    self.assertEqual(mk(f('a'), f('b')), mk(f('b'), f('a')))
    self.assertNotEqual(mk(f('a'), f('b')), mk(f('b'), f('a', 'desc')))
    self.assertEqual(
        mk(f('a', mk(f('x'), f('y')))),
        mk(f('a', mk(f('y'), f('x')))))
    self.assertNotEqual(
        mk(f('a', mk(f('x'), f('y')))),
        mk(f('a', mk(f('y'), f('x', 'desc')))))


class UpdateTablesTest(unittest.TestCase):
  def call(self, bq, schemas):
    tmp_dir = tempfile.mkdtemp(suffix='bq_util_test')
    try:
      bq_util.update_tables_cmd(bq, ArgsMock(tmp_dir, schemas))
    finally:
      shutil.rmtree(tmp_dir)

  def test_nothing(self):
    self.call(BigQueryMock({}), [])

  def test_create_tables(self):
    bq = BigQueryMock({})

    self.call(bq, [
      {
        'table_id': 'tab1',
        'description': 'Tab1 desc',
        'time_partitioning': True,
        'time_partitioning_exp_days': 90,
        'fields': [
          {'name': 'int', 'type': 'INTEGER'},
        ],
      },
      {
        'table_id': 'tab2',
        'description': 'Tab2 desc',
        'fields': [
          {'name': 'int', 'type': 'INTEGER'},
        ],
      },
    ])

    self.assertEqual([
      {
        'time_partitioning_exp_days': 90,
        'description': 'Tab1 desc',
        'table_id': 'tab1',
        'time_partitioning': True,
        'dataset_id': DATASET_ID,
        'schema': [{'type': 'INTEGER', 'name': 'int'}],
      },
      {
        'time_partitioning_exp_days': 0,
        'description': 'Tab2 desc',
        'table_id': 'tab2',
        'time_partitioning': False,
        'dataset_id': DATASET_ID,
        'schema': [{'type': 'INTEGER', 'name': 'int'}]
      },
    ], bq.create_table_calls)

  def test_noop_update(self):
    bq = BigQueryMock({
      'tab1': {
        'description': 'Tab1 desc',
        'timePartitioning': {
          'type': 'DAY',
          'expirationMs': '7776000000',
        },
        'schema': {
          'fields': [
            {'name': 'int', 'type': 'INTEGER'},
          ],
        },
      },
    })

    self.call(bq, [{
      'table_id': 'tab1',
      'description': 'Tab1 desc',
      'time_partitioning': True,
      'time_partitioning_exp_days': 90,
      'fields': [
        {'name': 'int', 'type': 'INTEGER'},
      ],
    }])
    self.assertEqual(0, len(bq.create_table_calls))
    self.assertEqual(0, len(bq.update_table_calls))

  def test_real_update(self):
    bq = BigQueryMock({
      'tab1': {
        'description': 'Tab1 desc',
        'timePartitioning': {
          'type': 'DAY',
          'expirationMs': '7776000000',
        },
        'schema': {
          'fields': [
            {'name': 'int', 'type': 'INTEGER'},
          ],
        },
      },
    })

    self.call(bq, [{
      'table_id': 'tab1',
      'description': 'Tab2 new desc',
      'time_partitioning': True,
      'time_partitioning_exp_days': 90,
      'fields': [
        {'name': 'int', 'type': 'INTEGER'},
        {'name': 'float', 'type': 'FLOAT'},
      ],
    }])
    self.assertEqual(0, len(bq.create_table_calls))
    self.assertEqual([
      {
        'dataset_id': 'fake-dataset',
        'description': 'Tab2 new desc',
        'schema': [
          {'name': 'int', 'type': 'INTEGER'},
          {'name': 'float', 'type': 'FLOAT'},
        ],
        'table_id': 'tab1',
        'time_partitioning': None,
        'time_partitioning_exp_days': None,
      }
    ], bq.update_table_calls)


if __name__ == '__main__':
  unittest.main()

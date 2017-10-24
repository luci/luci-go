#!/usr/bin/env python

"""Wrapper around 'bq' tool from Google Cloud SDK.

It automates some common tasks, like dataset and table creation and schema
changes.

It reads a definition of the desired state of tables from config files,
compares it to what's in BigQuery now, and performs necessary updates. It will
not execute any removals though. The user must execute removals manually, to
acknowledge the data loss.

Prerequisites:
  * 'bq' tool is in PATH.
  * Application default credentials are configured.
"""

import argparse
import ast
import collections
import difflib
import json
import logging
import subprocess
import sys
import tempfile


class Error(Exception):
  pass


class BigQueryError(Error):
  pass


class DefinitionError(Error):
  pass


class BigQuery(object):
  """A thin wrapper around 'bq' tool.

  Hides its CLI interface and argument marshaling. In theory, can be replaced
  with direct REST API calls. It will speed everything up significantly.

  All methods may raise BigQueryError.
  """

  def __init__(self, project_id):
    self.project_id = str(project_id)

  def _call(self, cmd, args, decode_json=True):
    full_cmd = [
      'bq',
      '--api_version', 'v2',  # we rely on particular format of JSON responses
      '--project_id', self.project_id,
      '--format', 'prettyjson',
      cmd
    ] + list(args)
    logging.debug('Calling %s', full_cmd)
    try:
      output = subprocess.check_output(full_cmd)
    except subprocess.CalledProcessError as exc:
      logging.debug('Failed with code %d: %s', exc.returncode)
      raise BigQueryError(exc.output.strip())
    logging.debug('Output:\n%s', output or '<empty>')
    if decode_json:
      return json.loads(output) if output else None
    return output

  def _tid(self, dataset_id, table_id):
    """Returns fully-qualified table ID."""
    return '%s:%s.%s' % (self.project_id, dataset_id, table_id)

  ### Datasets

  def list_datasets(self):
    """Returns a list of dataset IDs in the project."""
    reply = self._call('ls', ['%s:' % self.project_id]) or []
    return [d['datasetReference']['datasetId'] for d in reply]

  def create_dataset(self, dataset_id, description=''):
    # 'mk' returns "Dataset '...' successfully created." instead of JSON.
    self._call(
      'mk', ['--description', description, '-d', dataset_id],
      decode_json=False)

  ### Tables

  def list_tables(self, dataset_id):
    """Returns a list of table IDs within the dataset."""
    reply = self._call('ls', ['%s:%s.' % (self.project_id, dataset_id)]) or []
    return [t['tableReference']['tableId'] for t in reply]

  def get_table_metadata(self, dataset_id, table_id):
    """Returns information about an existing table (as a dict).

    The format of the dict matches 'bigquery#table' resource defined here:
    https://cloud.google.com/bigquery/docs/reference/rest/v2/tables
    """
    out = self._call('show', [self._tid(dataset_id, table_id)])
    assert out.get('kind') == 'bigquery#table'
    return out

  def create_table(
      self,
      dataset_id,
      table_id,
      schema,
      time_partitioning,
      time_partitioning_exp_days,
      description):
    """Creates a table (time partitioned or not).

    Args:
      dataset_id: ID of an existing dataset.
      table_id: ID of the table to create.
      schema: a dict with table fields description (in REST API format).
      time_partitioning: True to enable time partitioning.
      time_partitioning_exp_days: number of days to keep or 0 for forever.
      description: a friendly text description.
    """
    with tempfile.NamedTemporaryFile() as schema_tmp:
      json.dump(schema, schema_tmp)
      schema_tmp.flush()

      args = ['--schema', schema_tmp.name]
      if time_partitioning:
        args.extend(['--time_partitioning_type', 'DAY'])
        if time_partitioning_exp_days:
          args.extend([
            '--time_partitioning_expiration',
            str(time_partitioning_exp_days*24*3600),
          ])
      if description:
        args.extend(['--description', description])
      args.append(self._tid(dataset_id, table_id))

      self._call('mk', args, decode_json=False)

  def update_table(
      self,
      dataset_id,
      table_id,
      schema=None,
      time_partitioning=None,
      time_partitioning_exp_days=None,
      description=None):
    """Updates a table definition.

    Only non-None fields are updated.

    Args:
      dataset_id: ID of an existing dataset.
      table_id: ID of the table to create.
      schema: a dict with table fields description (in REST API format).
      time_partitioning: True to enable time partitioning.
      time_partitioning_exp_days: number of days to keep or 0 for forever.
      description: a friendly text description.
    """
    args = []

    schema_tmp = None
    if schema is not None:
      schema_tmp = tempfile.NamedTemporaryFile()
      json.dump(schema, schema_tmp)
      schema_tmp.flush()
      args.extend(['--schema', schema_tmp.name])

    if time_partitioning is not None:
      if not time_partitioning:
        raise BigQueryError('Removing time partitioning is not supported')
      args.extend(['--time_partitioning_type', 'DAY'])
      if time_partitioning_exp_days is not None:
        if time_partitioning_exp_days > 0:
          args.extend([
            '--time_partitioning_expiration',
              str(time_partitioning_exp_days*24*3600),
          ])
        else:
          args.extend(['--time_partitioning_expiration', '-1'])

    if description is not None:
      args.extend(['--description', description])

    if not args:
      return  # nothing to update

    args.append(self._tid(dataset_id, table_id))

    try:
      self._call('update', args, decode_json=False)
    finally:
      if schema_tmp:
        schema_tmp.close()


class Table(object):
  """A description of one table (including its schema).

  Includes only properties we will ever touch. For example, we don't use
  'expirationTime', and so it's completely omitted from this description. All
  omitted properties are left unchanged by updates.
  """

  def __init__(self, dataset_id, table_id):
    self.dataset_id = dataset_id
    self.table_id = table_id
    self.description = ''
    self.time_partitioning = False
    self.time_partitioning_exp_days = 0
    self.fields = TableFields()

  @classmethod
  def load_from_definition(cls, dataset_id, path):
    """Loads the table definition from Python literal eval file (*.schema).

    The file at 'path' is expected to contain python dict with following
    structure:
      {
        'table_id': '<must match table_id>',
        'description': '<arbitrary text description>',
        'time_partitioning': <True or False>,
        'time_partitioning_exp_days': <int with number of days to keep>,
        'schema': <TableFieldSchema list>,
      }

    Where <TableFieldSchema list> is a list:
      [
        {
          'name': '<field name>',
          'type': '<field type, e.g INTEGER or RECORD>',
          'mode': '<NULLABLE, REQUIRED or REPEATED (default is NULLABLE)>',
          'description': '<arbitrary text description>',
          'fields': <TableFieldSchema list with structure for RECORD fields>,
        },
        ...
      ]
    """
    with open(path, 'rt') as f:
      text = f.read()
    obj = ast.literal_eval(text)
    if not isinstance(obj, dict):
      raise DefinitionError('A schema document should be a dict')

    table_id = obj.pop('table_id', None)
    if not table_id or not isinstance(table_id, basestring):
      raise DefinitionError('"table_id" field must be a string')

    table = cls(dataset_id, table_id)

    description = obj.pop('description', '')
    if not isinstance(description, basestring):
      raise DefinitionError('"description" must be a string')
    table.description = description

    time_partitioning = obj.pop('time_partitioning', False)
    if not isinstance(time_partitioning, bool):
      raise DefinitionError('"time_partitioning" must be True or False')
    table.time_partitioning = time_partitioning

    time_partitioning_exp_days = obj.pop('time_partitioning_exp_days', 0)
    if not isinstance(time_partitioning_exp_days, int):
      raise DefinitionError('"time_partitioning_exp_days" must be an int')
    table.time_partitioning_exp_days = time_partitioning_exp_days

    fields = obj.pop('fields', [])
    if not isinstance(fields, list):
      raise DefinitionError('"fields" must be a list')
    table.fields = TableFields.load_from_definition(fields)

    if obj:
      raise DefinitionError(
          'Unknown table definition keys in %s: %s' % (path, ', '.join(obj)))

    return table

  @classmethod
  def load_from_bq(cls, bq, dataset_id, table_id):
    """Loads the table definition by fetching BigQuery table metadata."""
    meta = bq.get_table_metadata(dataset_id, table_id)
    table = cls(dataset_id, table_id)

    table.description = meta['description']

    partitioning = meta.get('timePartitioning')
    if partitioning:
      # BigQuery only supports daily partitioning currently.
      assert partitioning['type'] == 'DAY'
      table.time_partitioning = True
      ms = int(partitioning.get('expirationMs', '0'))
      table.time_partitioning_exp_days = ms / 1000 / 3600 / 24
      # Only 1 day resolution is actually supported.
      assert ms == table.time_partitioning_exp_days * 24 * 3600 * 1000

    # BQ format for fields schema is same as on-disk format we use in *.schema.
    table.fields = TableFields.load_from_definition(
        meta.get('schema', {}).get('fields', []))
    return table

  def __eq__(self, other):
    assert isinstance(other, Table)
    keys = (
      'dataset_id', 'table_id', 'description', 'time_partitioning',
      'time_partitioning_exp_days', 'fields')
    for k in keys:
      if getattr(self, k) != getattr(other, k):
        return False
    return True

  def __ne__(self, other):
    return not self == other

  def create(self, bq):
    """Creates this table in BigQuery."""
    bq.create_table(
      dataset_id=self.dataset_id,
      table_id=self.table_id,
      schema=self.fields.to_definition(),
      time_partitioning=self.time_partitioning,
      time_partitioning_exp_days=self.time_partitioning_exp_days,
      description=self.description)

  def apply_diff(self, bq, other):
    """Updates this table in BigQuery to match 'other'.

    Updates only changed fields. Prints diff to the console.
    """
    assert self.dataset_id == other.dataset_id
    assert self.table_id == other.table_id

    schema = None
    if self.fields != other.fields:
      print 'Schema diff:'
      print diff_table_fields(self.fields, other.fields)
      schema = other.fields.to_definition()

    time_partitioning = None
    if self.time_partitioning != other.time_partitioning:
      print 'Time partitioning: %s' % other.time_partitioning
      time_partitioning = other.time_partitioning

    time_partitioning_exp_days = None
    if self.time_partitioning_exp_days != other.time_partitioning_exp_days:
      val = (
          '%d days' if other.time_partitioning_exp_days else 'no expiration')
      print 'Time partitioning expiration: %s' % val
      time_partitioning_exp_days = other.time_partitioning_exp_days

    description = None
    if self.description != other.description:
      print 'Description: %s' % other.description
      description = other.description

    bq.update_table(
      dataset_id=self.dataset_id,
      table_id=self.table_id,
      schema=schema,
      time_partitioning=time_partitioning,
      time_partitioning_exp_days=time_partitioning_exp_days,
      description=description)


class TableFields(object):
  """A schema of a row or a structured field."""

  Field = collections.namedtuple('Field', 'name type mode description fields')

  # Supported field types, minus confusing aliases (e.g we always use BOOLEAN,
  # not BOOL).
  KNOWN_FIELD_TYPES = frozenset([
    'BOOLEAN',
    'BYTES',
    'DATE',
    'DATETIME',
    'FLOAT',
    'INTEGER',
    'RECORD',
    'STRING',
    'TIME',
    'TIMESTAMP',
  ])

  KNOWN_FIELD_MODES = frozenset(['NULLABLE', 'REQUIRED', 'REPEATED'])

  def __init__(self, fields=None):
    self.fields = list(fields or [])

  def __eq__(self, other):
    """Returns True if this schema is semantically identical to 'other'.

    Ignores order of fields, just like BigQuery does.
    """
    return {f.name: f for f in self.fields} == {f.name: f for f in other.fields}

  def __ne__(self, other):
    return not self == other

  @classmethod
  def load_from_definition(cls, fields_list):
    """Loads fields description from the python list."""
    obj = cls()

    for f in fields_list:
      if not isinstance(f, dict):
        raise DefinitionError('A field must be described in a dict')
      f = f.copy()

      name = f.pop('name', None)
      if not name or not isinstance(name, basestring):
        raise DefinitionError('Field name must be a string, not %r' % (name,))

      typ = f.pop('type', None)
      if typ not in cls.KNOWN_FIELD_TYPES:
        raise DefinitionError('Unknown field type %r' % (typ,))

      mode = f.pop('mode', 'NULLABLE')
      if mode not in cls.KNOWN_FIELD_MODES:
        raise DefinitionError('Unknown field mode %r' % (mode,))

      desc = f.pop('description', '')
      if not isinstance(desc, basestring):
        raise DefinitionError(
            'Field description must be a string, not %r' % (desc,))

      inner_fields_obj = None
      fields = f.pop('fields', [])
      if not isinstance(fields, list):
        raise DefinitionError(
            'Fields schema must be a list, not %r' % (fields,))
      if typ == 'RECORD':
        inner_fields_obj = TableFields.load_from_definition(fields)
      elif fields:
        raise DefinitionError('Only RECORD fields can have schema')

      if f:
        raise DefinitionError(
            'Unknown keys in field definition: %s' % ', '.join(f))

      obj.fields.append(cls.Field(name, typ, mode, desc, inner_fields_obj))

    return obj

  def to_definition(self):
    """Produces a JSONish dict with fields definition.

    The format of the dict matches REST API format, and on disk *.schema
    representation.
    """
    out = []
    for f in self.fields:
      d = {'name': f.name, 'type': f.type}
      if f.mode != 'NULLABLE':  # this is default
        d['mode'] = f.mode
      if f.description:
        d['description'] = f.description
      if f.fields:
        d['fields'] = f.fields.to_definition()
      out.append(d)
    return out


def diff_table_fields(old, new):
  """Returns text diff between two TableFields objects.

  Ignores order of fields.
  """
  def sort_fields(fields):
    assert isinstance(fields, list)
    fields.sort(key=lambda f: f['name'])
    for field in fields:
      if field.get('fields'):
        sort_fields(field.get('fields'))
    return fields

  def normalized_lines(fields):
    return json.dumps(
      sort_fields(fields.to_definition()),
      sort_keys=True,
      indent=2,
      separators=(',', ': ')).splitlines()

  d = difflib.Differ()
  return '\n'.join(d.compare(normalized_lines(old), normalized_lines(new)))


def update_tables_cmd(bq, args):
  print 'Ensuring that the dataset "%s" exists...' % args.dataset_id
  if args.dataset_id not in bq.list_datasets():
    bq.create_dataset(args.dataset_id)

  desired_tables = {}
  for path in getattr(args, 'schema-file'):
    t = Table.load_from_definition(args.dataset_id, path)
    desired_tables[t.table_id] = t

  # Create completely new tables.
  all_tables = bq.list_tables(args.dataset_id)
  for table_id in sorted(set(desired_tables) - set(all_tables)):
    print 'Creating table "%s"...' % table_id
    desired_tables[table_id].create(bq)

  # Update existing tables.
  for table_id in sorted(set(desired_tables) & set(all_tables)):
    print 'Fetching the state of the table "%s"...' % table_id
    old = Table.load_from_bq(bq, args.dataset_id, table_id)
    new = desired_tables[table_id]
    if old == new:
      print 'Table "%s" is up-to-date.' % table_id
    else:
      print 'Updating table "%s"...' % table_id
      old.apply_diff(bq, new)


def main(argv):
  parser = argparse.ArgumentParser(description='Big Query helper.')
  parser.set_defaults(verbose=False)
  parser.add_argument(
      '-p', '--project-id', metavar='PROJECT_ID', type=str,
      help='Cloud ProjectID that contains the dataset')
  parser.add_argument(
      '-d', '--dataset-id', metavar='DATASET_ID', type=str,
      help='Dataset ID to operate it (will be created if missing)')
  parser.add_argument(
      '-v', '--verbose', action='store_true', help='More logging')

  subparsers = parser.add_subparsers()
  update_args = subparsers.add_parser('update-tables')
  update_args.add_argument(
      'schema-file', metavar='SCHEMA_FILE', type=str, nargs='+',
      help='Path to a file with a table definition to apply')
  update_args.set_defaults(subcmd_func=update_tables_cmd)

  args = parser.parse_args(argv[1:])
  if not args.project_id:
    print '--project-id is required'
    parser.print_help()
    return 1
  if not args.dataset_id:
    print '--dataset-id is required'
    parser.print_help()
    return 1

  logging.basicConfig(level=logging.DEBUG if args.verbose else logging.INFO)

  try:
    return args.subcmd_func(BigQuery(args.project_id), args)
  except Error as exc:
    print >> sys.stderr, '-'*30 + ' ERROR ' + '-'*30
    print >> sys.stderr, str(exc)
    print >> sys.stderr, '-'*30 + ' ERROR ' + '-'*30
    return 1


if __name__ == '__main__':
  sys.exit(main(sys.argv))

#!/usr/bin/env python
# Copyright 2017 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

from setuptools import setup

setup(name='pants',
      version='1.2',
      description='Testing package (pants)',
      url='https://www.example.com/pants',
      packages=['pants'],
      depends=['shirt==3.14'],
     )

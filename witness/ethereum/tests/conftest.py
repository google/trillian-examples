#  Copyright 2021 Google LLC. All Rights Reserved.
# 
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

import pytest

from helpers import get_random_hash


@pytest.fixture(scope="function", autouse=True)
def basic_deployment(fn_isolation):
  pass


@pytest.fixture(scope="module")
def witness_deployer(accounts):
  return accounts.at(get_random_hash()[:42], force=True)


@pytest.fixture(scope="module")
def nondeployer_account(accounts):
  return accounts.at(get_random_hash()[:42], force=True)


@pytest.fixture(scope="module")
def witness_utils(WitnessUtils, witness_deployer):
  return witness_deployer.deploy(WitnessUtils)


@pytest.fixture(scope="module")
def compact_range_witness(CompactRangeWitness, witness_deployer):
  return witness_deployer.deploy(CompactRangeWitness)


@pytest.fixture(scope="module")
def root_witness(RootWitness, witness_deployer):
  return witness_deployer.deploy(RootWitness)


@pytest.fixture
def get_fixture(request):

  def _get_fixture(name):
    return request.getfixturevalue(name)

  return _get_fixture

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

import brownie
import pytest
import random

from helpers import (
    byte_to_hex_string,
    assert_root_tree_id_has_default_values,
    HASH_A,
    HASH_B,
    HASH_AB,
    HASH_C,
    HASH_AB_C,
    HASH_D,
    HASH_CD,
    NODE_HASHES,
    ROOT_HASHES,
    ZERO_BYTE,
    COMPACT_TREES,
    SIGNATURE_1,
    SIGNATURE_2,
    TREE_ID_1,
    TREE_ID_2,
    TIMESTAMP_1,
    TIMESTAMP_2,
    TIMESTAMP_3,
    TIMESTAMP_4,
)


def test_initial_state(root_witness, witness_deployer):
  assert root_witness.owner() == witness_deployer.address
  assert_root_tree_id_has_default_values(root_witness, TREE_ID_1)
  assert_root_tree_id_has_default_values(root_witness, TREE_ID_2)


def test_update_fails_for_nonowner_sender(root_witness, nondeployer_account):
  with brownie.reverts("msg.sender is not owner"):
    root_witness.update(
        TREE_ID_1,
        [HASH_A],
        1,
        TIMESTAMP_1,
        SIGNATURE_1,
        {"from": nondeployer_account},
    )


def test_update_fails_for_nonincreasing_timestamp(root_witness):
  # initial timestamp is 0; first update's timestamp must be >0
  with brownie.reverts("timestamp must increase"):
    root_witness.update(
        TREE_ID_1,
        [],
        1,
        0,
        SIGNATURE_1,
    )
  # test failing update from a non-zero timestamp
  root_witness.update(
      TREE_ID_1,
      [],
      0,
      TIMESTAMP_2,
      SIGNATURE_2,
  )
  with brownie.reverts("timestamp must increase"):
    root_witness.update(
        TREE_ID_1,
        [],
        0,
        TIMESTAMP_1,
        SIGNATURE_1,
    )


def test_update_fails_for_decreasing_size(root_witness):
  # initial tree size is 0
  root_witness.update(
      TREE_ID_1,
      [HASH_A],
      123,
      TIMESTAMP_1,
      SIGNATURE_1,
  )
  with brownie.reverts("tree size can't decrease"):
    root_witness.update(
        TREE_ID_1,
        [],
        122,
        TIMESTAMP_2,
        SIGNATURE_2,
    )


def test_update_expects_empty_proof_if_size_doesnt_change(root_witness):
  with brownie.reverts("empty proof is expected when tree size stays the same"):
    root_witness.update(TREE_ID_2, [HASH_A], 0, TIMESTAMP_2, SIGNATURE_2)


@pytest.mark.parametrize(
    "proof",
    [
        ([HASH_A, HASH_B]),  # multi-element proof
        ([]),  # empty proof
    ])
def test_update_requires_single_element_proof_from_empty_tree(
    root_witness, proof):
  with brownie.reverts("for an empty tree, proofs must have length=1"):
    root_witness.update(TREE_ID_1, proof, 1, TIMESTAMP_1, SIGNATURE_1)


def test_update_succeeds(root_witness):
  # successful update from empty
  receipt = root_witness.update(TREE_ID_1, [HASH_A], 1, TIMESTAMP_1,
                                SIGNATURE_1)
  assert receipt.return_value == (HASH_A, 1, TIMESTAMP_1, SIGNATURE_1)
  assert root_witness.getTreeDataById(TREE_ID_1) == receipt.return_value
  assert len(receipt.events) == 1 and len(receipt.events["Updated"]) == 1
  assert receipt.events["Updated"][0].items() == [
      ("treeId", TREE_ID_1),
      ("newTreeData", receipt.return_value),
      ("consistencyProof", (HASH_A,)),
  ]
  # successful update for populated tree
  receipt = root_witness.update(TREE_ID_1, [HASH_B], 2, TIMESTAMP_2,
                                SIGNATURE_2)
  assert receipt.return_value == (HASH_AB, 2, TIMESTAMP_2, SIGNATURE_2)
  assert root_witness.getTreeDataById(TREE_ID_1) == receipt.return_value
  assert len(receipt.events) == 1 and len(receipt.events["Updated"]) == 1
  assert receipt.events["Updated"][0].items() == [
      ("treeId", TREE_ID_1),
      ("newTreeData", receipt.return_value),
      ("consistencyProof", (HASH_B,)),
  ]


def test_handles_different_tree_ids(root_witness):
  assert_root_tree_id_has_default_values(root_witness, TREE_ID_1)
  assert_root_tree_id_has_default_values(root_witness, TREE_ID_2)
  assert HASH_A == root_witness.update(TREE_ID_1, [HASH_A], 10, TIMESTAMP_1,
                                       SIGNATURE_1).return_value[0]
  # verify updates to one ID don't affect the other
  assert_root_tree_id_has_default_values(root_witness, TREE_ID_2)
  # verify second ID can be updated without affecting first
  assert (HASH_B, 23, TIMESTAMP_2,
          SIGNATURE_2) == root_witness.update(TREE_ID_2, [HASH_B], 23,
                                              TIMESTAMP_2,
                                              SIGNATURE_2).return_value
  assert (HASH_A, 10, TIMESTAMP_1,
          SIGNATURE_1) == root_witness.getTreeDataById(TREE_ID_1)


def test_empty_update_succeeds(root_witness):
  # works for an empty tree
  expected_tree_data = (ZERO_BYTE, 0, TIMESTAMP_1, SIGNATURE_1)
  assert root_witness.update(TREE_ID_1, [], 0, TIMESTAMP_1,
                             SIGNATURE_1).return_value == expected_tree_data
  # works for a nonempty tree
  assert root_witness.update(TREE_ID_1, [HASH_A], 12, TIMESTAMP_2,
                             SIGNATURE_1).return_value[0] == HASH_A
  expected_tree_data = (HASH_A, 12, TIMESTAMP_3, SIGNATURE_2)
  assert root_witness.update(TREE_ID_1, [], 12, TIMESTAMP_3,
                             SIGNATURE_2).return_value == expected_tree_data


def test_batch_update_succeeds(root_witness):
  assert_root_tree_id_has_default_values(root_witness, TREE_ID_1)
  assert_root_tree_id_has_default_values(root_witness, TREE_ID_2)

  receipt = root_witness.batchUpdate([(TREE_ID_1, [HASH_A], 1, TIMESTAMP_1,
                                       SIGNATURE_1)])
  assert len(receipt.return_value) == 1
  assert receipt.return_value[0] == (HASH_A, 1, TIMESTAMP_1, SIGNATURE_1)
  assert receipt.return_value[0] == root_witness.getTreeDataById(TREE_ID_1)

  receipt = root_witness.batchUpdate([
      (TREE_ID_1, [HASH_B], 2, TIMESTAMP_2, SIGNATURE_2),
      (TREE_ID_2, [HASH_C], 5, TIMESTAMP_1, SIGNATURE_1)
  ])
  assert len(receipt.return_value) == 2
  assert receipt.return_value[0] == (HASH_AB, 2, TIMESTAMP_2, SIGNATURE_2)
  assert receipt.return_value[0] == root_witness.getTreeDataById(TREE_ID_1)
  assert receipt.return_value[1] == (HASH_C, 5, TIMESTAMP_1, SIGNATURE_1)
  assert receipt.return_value[1] == root_witness.getTreeDataById(TREE_ID_2)

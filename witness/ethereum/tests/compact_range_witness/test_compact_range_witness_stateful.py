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
    merge_compact_range,
    build_range,
    assert_cr_tree_id_has_default_values,
    HASH_A,
    HASH_B,
    HASH_AB,
    HASH_C,
    HASH_AB_C,
    HASH_D,
    HASH_CD,
    NODE_HASHES,
    ROOT_HASHES,
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


def test_initial_state(compact_range_witness, witness_deployer):
  assert compact_range_witness.owner() == witness_deployer.address
  assert_cr_tree_id_has_default_values(compact_range_witness, TREE_ID_1)
  assert_cr_tree_id_has_default_values(compact_range_witness, TREE_ID_2)


def test_update_with_single_element_succeeds(compact_range_witness):
  update_txn_receipt = compact_range_witness.update(TREE_ID_1, [HASH_A], 1,
                                                    TIMESTAMP_1, SIGNATURE_1)
  assert (len(update_txn_receipt.events) == 1 and
          len(update_txn_receipt.events["Updated"]) == 1)
  assert update_txn_receipt.events["Updated"][0].items() == [
      ("treeId", TREE_ID_1),
      ("root", HASH_A),
      ("treeSize", 1),
      ("timestamp", TIMESTAMP_1),
      ("appendedCompactRange", (HASH_A,)),
      ("signature", SIGNATURE_1),
  ]
  assert update_txn_receipt.return_value == HASH_A
  assert compact_range_witness.getRootForTreeId(TREE_ID_1) == HASH_A
  assert compact_range_witness.getTreeDataById(TREE_ID_1) == ([HASH_A
                                                              ], SIGNATURE_1, 1,
                                                              TIMESTAMP_1)
  assert_cr_tree_id_has_default_values(compact_range_witness, TREE_ID_2)


def test_empty_update_succeeds(compact_range_witness):
  compact_range_witness.update(TREE_ID_1, [HASH_A], 1, TIMESTAMP_1, SIGNATURE_1)
  assert compact_range_witness.update(TREE_ID_1, [], 1, TIMESTAMP_2,
                                      SIGNATURE_2).return_value == HASH_A
  # assert root, range, and size values stayed the same.
  # signature and timestamp should be updated to new values.
  assert compact_range_witness.getRootForTreeId(TREE_ID_1) == HASH_A
  assert compact_range_witness.getTreeDataById(TREE_ID_1) == ([HASH_A
                                                              ], SIGNATURE_2, 1,
                                                              TIMESTAMP_2)


def test_multiple_updates_succeeds(compact_range_witness):
  compact_range_witness.update(TREE_ID_2, [HASH_A], 1, TIMESTAMP_1, SIGNATURE_1)
  update_txn_receipt = compact_range_witness.update(TREE_ID_2, [HASH_B], 2,
                                                    TIMESTAMP_2, SIGNATURE_2)
  assert update_txn_receipt.events["Updated"][0].items() == [
      ("treeId", TREE_ID_2),
      ("root", HASH_AB),
      ("treeSize", 2),
      ("timestamp", TIMESTAMP_2),
      ("appendedCompactRange", (HASH_B,)),
      ("signature", SIGNATURE_2),
  ]
  assert update_txn_receipt.return_value == HASH_AB
  assert compact_range_witness.getTreeDataById(TREE_ID_2) == ([HASH_AB
                                                              ], SIGNATURE_2, 2,
                                                              TIMESTAMP_2)
  assert compact_range_witness.getRootForTreeId(TREE_ID_2) == HASH_AB

  assert compact_range_witness.update(TREE_ID_2, [HASH_C], 3, TIMESTAMP_3,
                                      SIGNATURE_2).return_value == HASH_AB_C
  assert compact_range_witness.getTreeDataById(TREE_ID_2) == ([HASH_AB, HASH_C
                                                              ], SIGNATURE_2, 3,
                                                              TIMESTAMP_3)
  assert_cr_tree_id_has_default_values(compact_range_witness, TREE_ID_1)


def test_update_multiple_tree_ids_succeeds(compact_range_witness):
  assert_cr_tree_id_has_default_values(compact_range_witness, TREE_ID_1)
  assert_cr_tree_id_has_default_values(compact_range_witness, TREE_ID_2)
  # Update TREE_ID_1
  assert (compact_range_witness.update(TREE_ID_1, [HASH_A], 1, TIMESTAMP_1,
                                       SIGNATURE_1).return_value == HASH_A)
  assert (compact_range_witness.update(TREE_ID_1, [HASH_B], 2, TIMESTAMP_2,
                                       SIGNATURE_1).return_value == HASH_AB)
  # Update TREE_ID_2
  assert (compact_range_witness.update(TREE_ID_2, [HASH_C], 2, TIMESTAMP_3,
                                       SIGNATURE_2).return_value == HASH_C)
  assert (compact_range_witness.update(TREE_ID_2, [HASH_D], 3, TIMESTAMP_4,
                                       SIGNATURE_2).return_value == HASH_CD)
  # Verify TREE_ID_1 wasn't affected
  assert compact_range_witness.getRootForTreeId(TREE_ID_1) == HASH_AB
  assert compact_range_witness.getTreeDataById(TREE_ID_1)[3] == TIMESTAMP_2


def test_update_fails_with_bad_sender(compact_range_witness,
                                      nondeployer_account):
  with brownie.reverts("msg.sender is not owner"):
    compact_range_witness.update(
        TREE_ID_1,
        [HASH_A],
        1,
        TIMESTAMP_1,
        SIGNATURE_1,
        {"from": nondeployer_account},
    )


def test_update_fails_if_tree_size_decreases(compact_range_witness):
  compact_range_witness.update(
      TREE_ID_1,
      [HASH_A],
      1,
      TIMESTAMP_1,
      SIGNATURE_1,
  )
  with brownie.reverts("tree size must not decrease"):
    compact_range_witness.update(TREE_ID_1, [HASH_B], 0, TIMESTAMP_2,
                                 SIGNATURE_1)


def test_update_fails_if_range_and_size_dont_correspond(compact_range_witness):
  with brownie.reverts(
      "right range bounds and number of hashes don't correspond"):
    compact_range_witness.update(TREE_ID_1, [], 1, TIMESTAMP_1, SIGNATURE_1)


def test_update_fails_if_size_stays_same_but_range_isnt_empty(
    compact_range_witness):
  with brownie.reverts(
      "when tree size stays the same, rightRange must be empty"):
    compact_range_witness.update(TREE_ID_1, [HASH_A], 0, TIMESTAMP_1,
                                 SIGNATURE_1)


def test_update_fails_if_timestamp_doesnt_increase(compact_range_witness):
  assert (compact_range_witness.update(TREE_ID_1, [HASH_A], 1, TIMESTAMP_1,
                                       SIGNATURE_1).return_value == HASH_A)
  with brownie.reverts("timestamp may only increase"):
    assert compact_range_witness.update(TREE_ID_1, [HASH_B], 2, TIMESTAMP_1,
                                        SIGNATURE_1)


@pytest.mark.parametrize("max_idx", list(range(len(NODE_HASHES[0]) + 1)))
def test_update_golden_ranges_succeeds(compact_range_witness, max_idx):
  assert_cr_tree_id_has_default_values(compact_range_witness, TREE_ID_1)
  for i in range(max_idx):
    compact_range_witness.update(
        TREE_ID_1,
        [byte_to_hex_string(NODE_HASHES[0][i])],
        i + 1,
        TIMESTAMP_1 + i,
        SIGNATURE_1,
    )
  assert compact_range_witness.getRootForTreeId(
      TREE_ID_1) == byte_to_hex_string(ROOT_HASHES[max_idx])
  assert compact_range_witness.getTreeDataById(TREE_ID_1)[0] == [
      byte_to_hex_string(h) for h in COMPACT_TREES[max_idx]
  ]


@pytest.mark.parametrize("seed", list(range(250)))
def test_update_randomly_succeeds(compact_range_witness, seed):
  assert_cr_tree_id_has_default_values(compact_range_witness, TREE_ID_1)

  random.seed(seed)
  size = random.randrange(2, 10000)
  midpoint = random.randrange(1, size)
  left = build_range(0, midpoint)
  right = build_range(midpoint, size)

  compact_range_witness.update(TREE_ID_1, left, midpoint, TIMESTAMP_1,
                               SIGNATURE_1)
  assert compact_range_witness.getTreeDataById(TREE_ID_1)[0] == left

  compact_range_witness.update(TREE_ID_1, right, size, TIMESTAMP_2, SIGNATURE_1)
  assert compact_range_witness.getTreeDataById(
      TREE_ID_1)[0] == merge_compact_range(0, left, midpoint, right, size)

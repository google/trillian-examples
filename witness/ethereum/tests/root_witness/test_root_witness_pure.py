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

import math
import random
import brownie
from helpers import (
    EMPTY_HASH,
    HASH_A,
    HASH_B,
    HASH_AB,
    HASH_AB_C,
    HASH_A_BC,
    HASH_C,
    HASH_D,
    HASH_CD,
    HASH_AB_CD,
)
import pytest

incorrect_proof_len_error = ("got incorrect proof length based on oldSize and "
                             "newSize")


@pytest.mark.parametrize("old_root, old_size, proof, new_size, new_root", [
    (HASH_A, 1, [HASH_B], 2, HASH_AB),
    (HASH_A, 1, [HASH_B, HASH_C], 3, HASH_AB_C),
    (HASH_AB, 6, [HASH_B, HASH_C, HASH_A], 7, HASH_A_BC),
    (HASH_AB, 10, [HASH_B, HASH_C, HASH_A], 11, HASH_A_BC),
    (HASH_AB, 1026, [HASH_B, HASH_C, HASH_A], 1027, HASH_A_BC),
])
def test_consumeConsistencyProof(root_witness, old_root, old_size, proof,
                                 new_size, new_root):
  assert new_root == root_witness.consumeConsistencyProof(
      old_root, old_size, proof, new_size)


@pytest.mark.parametrize(
    "old_root, old_size, proof, new_size, error",
    [
        (HASH_A, 1, [], 2, incorrect_proof_len_error),
        (HASH_A, 1, [HASH_B, HASH_C], 2, incorrect_proof_len_error),
        (HASH_A, 1, [HASH_B, HASH_C, HASH_D], 3, incorrect_proof_len_error),
        # oldHash should be AB instead of AB_C
        (HASH_AB_C, 3, [HASH_B, HASH_A, HASH_AB_C], 4,
         "consistency proof did not correspond with oldRoot"),
    ])
def test_consumeConsistencyProof_errors(root_witness, old_root, old_size, proof,
                                        new_size, error):
  with brownie.reverts(error):
    root_witness.consumeConsistencyProof(old_root, old_size, proof, new_size)


@pytest.mark.parametrize("n, expected", [
    (0, 0),
    (1, 0),
    (2, 1),
    (3, 0),
    (4, 2),
    (594, 1),
    (5220066665488, 4),
    (2**32, 32),
    (2**255, 255),
])
def test_getTrailingZeros(root_witness, n, expected):
  assert root_witness.getTrailingZeros(n) == expected


@pytest.mark.parametrize("n, expected", [
    (0, 0),
    (1, 1),
    (2, 2),
    (4, 3),
    (5, 3),
    (35, 6),
    (2**256 - 1, 256),
])
def test_minLen(root_witness, n, expected):
  assert root_witness.minLen(n) == expected


@pytest.mark.parametrize("index, size, inner,border", [
    (0, 2, 1, 0),
    (0, 10, 4, 0),
    (9, 11, 2, 1),
])
def test_decompInclProof(root_witness, index, size, inner, border):
  assert root_witness.decompInclProof(index, size) == (inner, border)

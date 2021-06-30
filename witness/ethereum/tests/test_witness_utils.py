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
import math
import random
import brownie
from helpers import (
    HASH_A,
    HASH_B,
    HASH_AB,
    HASH_AB_C,
    HASH_C,
    HASH_D,
    HASH_CD,
    HASH_AB_CD,
)


@pytest.mark.parametrize(
    "left, right, expected",
    [
        (HASH_A, HASH_B, HASH_AB),
        (HASH_AB, HASH_C, HASH_AB_C),
        (HASH_C, HASH_D, HASH_CD),
        (HASH_AB, HASH_CD, HASH_AB_CD),
        (
            "0x54a6483b8aca55c9df2a35baf71d9965ddfd623468d81d51229bd5eb7d1e1c1b",
            "0xc8ef5412a5b821bf9c91dceb09e6cbc2705aed7f205c6aabaa7fde5f20f9764a",
            "0xb1d1843bd27b583bace49186ef8d0eb7bfac7eb44bd77619da6708f1248343ab",
        ),
    ],
)
def test_hashToParent(witness_utils, left, right, expected):
  assert witness_utils.hashToParent(left, right) == expected


@pytest.mark.parametrize(
    "n, expectedMsb",
    [
        (1, 0),
        (2, 1),
        (3, 1),
        (32, 5),
        (35, 5),
        (2**60, 60),
        (2**256 - 1, 255),
        (2**256 - 2, 255),
        (2**256 - 1 - 2**255, 254),
    ],
)
def test_mostSignificantBit(witness_utils, n, expectedMsb):
  assert witness_utils.mostSignificantBit(n) == expectedMsb


@pytest.mark.parametrize("seed", list(range(50)))
def test_mostSignificantBit_randomly(witness_utils, seed):
  random.seed(seed)
  n = random.randrange(2**256)
  assert witness_utils.mostSignificantBit(n) == math.floor(math.log(n, 2))


@pytest.mark.parametrize(
    "n, expectedCount",
    [
        (0, 0),
        (1, 1),
        (2, 1),
        (3, 2),
        (32, 1),
        (35, 3),
        (2**256 - 1, 256),
        (2**256 - 2, 255),
    ],
)
def test_countSetBits(witness_utils, n, expectedCount):
  assert witness_utils.countSetBits(n) == expectedCount

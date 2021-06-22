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
    HASH_C,
    HASH_D,
    HASH_CD,
    HASH_AB_CD,
)
import pytest


@pytest.mark.parametrize(
    "compactRange, expected",
    [([], EMPTY_HASH), ([HASH_A], HASH_A), ([HASH_A, HASH_B], HASH_AB)],
)
def test_calcMerkleRoot(compact_range_witness, compactRange, expected):
  assert compact_range_witness.calcMerkleRoot(compactRange) == expected


@pytest.mark.parametrize(
    "leftRangeStart, leftRange, leftRangeEnd, rightRange, rightRangeEnd, expected",
    [
        (0, [], 0, [HASH_A], 1, [HASH_A]),
        (0, [HASH_A], 1, [HASH_B], 2, [HASH_AB]),
        (0, [HASH_A], 2, [HASH_B], 3, [HASH_A, HASH_B]),
        (0, [HASH_AB, HASH_C], 3, [HASH_D], 4, [HASH_AB_CD]),
        (1, [HASH_A], 2, [HASH_B], 3, [HASH_A, HASH_B]),
        (1, [HASH_A], 2, [HASH_B], 4, [HASH_A, HASH_B]),
    ],
)
def test_mergeCompactRanges(
    compact_range_witness,
    leftRangeStart,
    leftRange,
    leftRangeEnd,
    rightRange,
    rightRangeEnd,
    expected,
):
  assert (compact_range_witness.mergeCompactRanges(leftRangeStart, leftRange,
                                                   leftRangeEnd, rightRange,
                                                   rightRangeEnd) == expected)


@pytest.mark.parametrize(
    "leftRangeStart, leftRange, leftRangeEnd, rightRange, rightRangeEnd, expectedError",
    [
        (
            0,
            [HASH_A],
            0,
            [],
            0,
            "left range bounds and number of hashes don't correspond",
        ),
        (0, [], 0, [], 1, "right range bounds and number of hashes don't correspond"),
        (0, [], 0, [], 0, "right range must be non-empty"),
    ],
)
def test_mergeCompactRanges_fails_on_bad_args(
    compact_range_witness,
    leftRangeStart,
    leftRange,
    leftRangeEnd,
    rightRange,
    rightRangeEnd,
    expectedError,
):
  with brownie.reverts(expectedError):
    assert compact_range_witness.mergeCompactRanges(leftRangeStart, leftRange,
                                                    leftRangeEnd, rightRange,
                                                    rightRangeEnd)


@pytest.mark.parametrize(
    "begin, end, expected",
    [
        (0, 0, 0),
        (0, 2, 1),
        (0, 3, 2),
        (0, 4, 1),
        (1, 3, 2),
        (3, 7, 3),
        (3, 17, 4),
        (4, 28, 4),
        (8, 24, 2),
        (8, 28, 3),
        (11, 25, 4),
        (31, 45, 4),
        (1, 2, 1),
        (6, 29, 5),
        (0, 29, 4),
        (0, 32, 1),
        (91, 92, 1),
        (91, 100, 3),
    ],
)
def test_getExpectedNumHashesInCompactRange(compact_range_witness, begin, end,
                                            expected):
  assert compact_range_witness.getExpectedNumHashesInCompactRange(
      begin, end) == expected


@pytest.mark.parametrize(
    "left, leftEndIdx, seed, right, rightStartIdx, merged",
    [
        ([], 0, "0x1", [], 0, ["0x1"]),
        (["0x1"], 1, "0x2", [], 0, ["0x1", "0x2"]),
        ([], 0, "0x1", ["0x2"], 0, ["0x1", "0x2"]),
        (["0x1"], 1, "0x2", ["0x3"], 0, ["0x1", "0x2", "0x3"]),
        (
            ["0x1", "0x2", "0x3"],
            2,
            "0x4",
            ["0x5", "0x6", "0x7"],
            1,
            ["0x1", "0x2", "0x4", "0x6", "0x7"],
        ),
    ],
)
def test_concatRanges(compact_range_witness, left, leftEndIdx, seed, right,
                      rightStartIdx, merged):
  assert compact_range_witness.concatRanges(left, leftEndIdx, seed, right,
                                            rightStartIdx) == merged

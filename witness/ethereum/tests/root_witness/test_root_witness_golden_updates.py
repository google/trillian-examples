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
from helpers import (
    assert_root_tree_id_has_default_values,
    SIGNATURE_1,
    SIGNATURE_2,
    TREE_ID_1,
    TIMESTAMP_1,
    TIMESTAMP_2,
)

# Goldens based on https://github.com/google/trillian/blob/master/merkle/logverifier/log_verifier_test.go#L68

roots = [
    "0x6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d",
    "0xfac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125",
    "0xaeb6bcfe274b70a14fb067a5e5578264db0fa9b51af5e0ba159158f329e06e77",
    "0xd37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7",
    "0x4e3bbb1f7b478dcfe71fb631631519a3bca12c9aefca1612bfce4c13a86264d4",
    "0x76e67dadbcdf1e10e1b74ddc608abd2f98dfb16fbce75277b5232a127f2087ef",
    "0xddb89be403809e325750d3d263cd78929c2942b7942a34b77e122c9594a74c8c",
    "0x5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328",
]
consistency_proofs = [
    (1, 1, []),
    (
        1,
        8,
        [
            "0x96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7",
            "0x5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e",
            "0x6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4",
        ],
    ),
    (
        6,
        8,
        [
            "0x0ebc5d3437fbe2db158b9f126a1d118e308181031d0a949f8dededebc558ef6a",
            "0xca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0",
            "0xd37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7",
        ],
    ),
    (
        2,
        5,
        [
            "0x5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e",
            "0xbc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b",
        ],
    ),
]


@pytest.mark.parametrize("old_size, new_size, proof", consistency_proofs)
def test_golden_updates(root_witness, old_size, new_size, proof):
  assert_root_tree_id_has_default_values(root_witness, TREE_ID_1)
  old_root = roots[old_size - 1]
  new_root = roots[new_size - 1]
  assert root_witness.update(TREE_ID_1, [old_root], old_size, TIMESTAMP_1,
                             SIGNATURE_1).return_value == (old_root, old_size,
                                                           TIMESTAMP_1,
                                                           SIGNATURE_1)
  receipt = root_witness.update(TREE_ID_1, proof, new_size, TIMESTAMP_2,
                                SIGNATURE_2)
  assert receipt.return_value == (new_root, new_size, TIMESTAMP_2, SIGNATURE_2)
  assert root_witness.getTreeDataById(TREE_ID_1) == receipt.return_value

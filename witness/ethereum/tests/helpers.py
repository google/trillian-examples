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

from hashlib import sha256
import math
import random


def get_random_hash():
  random.seed()
  return hash_val(random.randrange(2**256).to_bytes(32, "big"))


def hash_val(val):
  return "0x" + sha256(val).hexdigest()


def byte_to_hex_string(b):
  return "0x" + b.hex()


def hex_string_to_bytes(h):
  if h.startswith("0x"):
    h = h[2:]
  return bytes.fromhex(h)


def hash_to_leaf(val):
  return hash_val(bytes.fromhex("00") + val)


def hash_to_parent(left, right):
  return hash_val(
      bytes.fromhex("01") + hex_string_to_bytes(left) +
      hex_string_to_bytes(right))


def calc_root_from_leaves(leaves):
  if len(leaves) == 0:
    return "0x" + sha256(b"").hexdigest()
  if len(leaves) == 1:
    return leaves[0]
  frontier = leaves
  while len(frontier) > 1:
    next_layer = []
    for i in range(math.floor((len(frontier) + 1) / 2)):
      next_layer.append(frontier[i * 2] if len(frontier) <= i * 2 +
                        1 else hash_to_parent(frontier[i * 2], frontier[i * 2 +
                                                                        1]))
    frontier = next_layer
  return frontier[0]


def merge_compact_range(left_start, left, left_end, right, right_end):
  seed = right[0]
  right = right[1:]
  for merge_height in range(256):
    num_covered_by_seed = 1 << merge_height
    seed_idx = left_end >> merge_height
    seed_covered_range_start = seed_idx * num_covered_by_seed
    if seed_idx & 1 == 0:
      # Right merge
      if right_end - seed_covered_range_start < 2 * num_covered_by_seed:
        break
      if ((num_covered_by_seed - 1) & left_end) == 0:
        continue
      seed = hash_to_parent(seed, right[0])
      right = right[1:]
    else:
      # Left merge
      if seed_covered_range_start - left_start < num_covered_by_seed:
        break
      seed = hash_to_parent(left[-1], seed)
      left = left[:-1]
  return left + [seed] + right


def build_range(start, end):
  left = []
  for i in range(start, end):
    left = merge_compact_range(start, left, i,
                               [hash_to_leaf(i.to_bytes(32, "big"))], i + 1)
  return left


def assert_cr_tree_id_has_default_values(compact_range_witness, tree_id):
  # Note: this method only calls view methods
  compact_range, signature, size, timestamp = compact_range_witness.getTreeDataById(
      tree_id)
  assert not compact_range
  assert signature == ZERO_BYTE
  assert size == 0
  assert timestamp == 0
  assert compact_range_witness.getRootForTreeId(tree_id) == EMPTY_HASH


def assert_root_tree_id_has_default_values(root_witness, tree_id):
  # Note: this method only calls view methods
  root, size, timestamp, signature = root_witness.getTreeDataById(tree_id)
  assert signature == ZERO_BYTE
  assert size == 0
  assert timestamp == 0
  assert root == ZERO_BYTE


EMPTY_HASH = hash_val(b"")
HASH_A = hash_to_leaf(b"hash a")
HASH_B = hash_to_leaf(b"hash b")
HASH_AB = hash_to_parent(HASH_A, HASH_B)
HASH_C = hash_to_leaf(b"hash c")
HASH_AB_C = hash_to_parent(HASH_AB, HASH_C)
HASH_BC = hash_to_parent(HASH_B, HASH_C)
HASH_A_BC = hash_to_parent(HASH_A, HASH_BC)
HASH_D = hash_to_leaf(b"hash d")
HASH_CD = hash_to_parent(HASH_C, HASH_D)
HASH_AB_CD = hash_to_parent(HASH_AB, HASH_CD)
ZERO_BYTE = "0x0"
SIGNATURE_1 = hash_val(b"signature 1")
SIGNATURE_2 = hash_val(b"signature 2")
TREE_ID_1 = hash_val(b"tree id 1")
TREE_ID_2 = hash_val(b"tree id 2")
TIMESTAMP_1 = 1613100131
TIMESTAMP_2 = 1613958814
TIMESTAMP_3 = 1613958815
TIMESTAMP_4 = 1613958816

NODE_HASHES = [[bytes.fromhex(nodehash) for nodehash in layer] for layer in [
    [
        "6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d",
        "96a296d224f285c67bee93c30f8a309157f0daa35dc5b87e410b78630a09cfc7",
        "0298d122906dcfc10892cb53a73992fc5b9f493ea4c9badb27b791b4127a7fe7",
        "07506a85fd9dd2f120eb694f86011e5bb4662e5c415a62917033d4a9624487e7",
        "bc1a0643b12e4d2d7c77918f44e0f4f79a838b6cf9ec5b5c283e1f4d88599e6b",
        "4271a26be0d8a84f0bd54c8c302e7cb3a3b5d1fa6780a40bcce2873477dab658",
        "b08693ec2e721597130641e8211e7eedccb4c26413963eee6c1e2ed16ffb1a5f",
        "46f6ffadd3d06a09ff3c5860d2755c8b9819db7df44251788c7d8e3180de8eb1",
    ],
    [
        "fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125",
        "5f083f0a1a33ca076a95279832580db3e0ef4584bdff1f54c8a360f50de3031e",
        "0ebc5d3437fbe2db158b9f126a1d118e308181031d0a949f8dededebc558ef6a",
        "ca854ea128ed050b41b35ffc1b87b8eb2bde461e9e3b5596ece6b9d5975a0ae0",
    ],
    [
        "d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7",
        "6b47aaf29ee3c2af9af889bc1fb9254dabd31177f16232dd6aab035ca39bf6e4",
    ],
    ["5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328"],
]]

ROOT_HASHES = [
    bytes.fromhex(n) for n in [
        EMPTY_HASH[2:],
        "6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d",
        "fac54203e7cc696cf0dfcb42c92a1d9dbaf70ad9e621f4bd8d98662f00e3c125",
        "aeb6bcfe274b70a14fb067a5e5578264db0fa9b51af5e0ba159158f329e06e77",
        "d37ee418976dd95753c1c73862b9398fa2a2cf9b4ff0fdfe8b30cd95209614b7",
        "4e3bbb1f7b478dcfe71fb631631519a3bca12c9aefca1612bfce4c13a86264d4",
        "76e67dadbcdf1e10e1b74ddc608abd2f98dfb16fbce75277b5232a127f2087ef",
        "ddb89be403809e325750d3d263cd78929c2942b7942a34b77e122c9594a74c8c",
        "5dc9da79a70659a9ad559cb701ded9a2ab9d823aad2f4960cfe370eff4604328",
    ]
]

COMPACT_TREES = [
    [],
    [NODE_HASHES[0][0]],
    [NODE_HASHES[1][0]],
    [NODE_HASHES[1][0], NODE_HASHES[0][2]],
    [NODE_HASHES[2][0]],
    [NODE_HASHES[2][0], NODE_HASHES[0][4]],
    [NODE_HASHES[2][0], NODE_HASHES[1][2]],
    [NODE_HASHES[2][0], NODE_HASHES[1][2], NODE_HASHES[0][6]],
    [NODE_HASHES[3][0]],
]

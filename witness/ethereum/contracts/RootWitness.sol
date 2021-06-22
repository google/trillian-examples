// Copyright 2021 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// SPDX-License-Identifier: Apache-2.0

pragma solidity ^0.8.0;

import {WitnessUtils} from "./WitnessUtils.sol";

/// @author sinasabet@google.com
/// @title Witness for tracking external merkle trees using consistency proofs.
contract RootWitness is WitnessUtils {
    /// @dev struct to encapsulate data for a specific tree.
    struct TreeData {
        bytes32 root;
        uint256 size;
        uint256 timestamp;
        bytes32 signature;
    }

    /// @dev struct to encapsulate params of a single update call
    struct UpdateData {
      bytes32 treeId;
      bytes32[] proof;
      uint256 size;
      uint256 timestamp;
      bytes32 signature;
    }

    /// @dev emits information related to an update method call.
    event Updated(
        bytes32 treeId,
        TreeData newTreeData,
        bytes32[] consistencyProof
    );

    address public owner;
    /// @dev mapping from treeId to that tree's data.
    mapping(bytes32 => TreeData) public treeDatas;

    constructor() {
        owner = msg.sender;
    }

    /// @dev getter for entire treeData value of a specific treeId.
    function getTreeDataById(bytes32 treeId)
        public
        view
        returns (TreeData memory)
    {
        return treeDatas[treeId];
    }

    /// @dev handler for batch updates.
    function batchUpdate(UpdateData[] calldata updateDatas)
      external
      returns
      (TreeData[] memory)
    {
      TreeData[] memory results = new TreeData[](updateDatas.length);
      for (uint256 i = 0; i < updateDatas.length; i++) {
        UpdateData calldata data = updateDatas[i];
        results[i] = update(
          data.treeId,
          data.proof,
          data.size,
          data.timestamp,
          data.signature
        );
      }
      return results;
    }

    /// Handles an update for the witness for a specific treeId.
    /// @param treeId the id of the tree the update is for.
    /// @param consistencyProof list of hashes making up the consistency proof.
    /// @param size the size of the updated tree.
    ///   Corresponds to end index of the right range.
    /// @param timestamp integer timestamp of the update.
    /// @param signature raw signature over the marshalled bytes of new root.
    /// @dev merge rightRange into compactRange.
    /// @return treeData updated treeData entry for treeId.
    function update(
        bytes32 treeId,
        bytes32[] calldata consistencyProof,
        uint256 size,
        uint256 timestamp,
        bytes32 signature
    ) public returns (TreeData memory) {
        require(owner == msg.sender, "msg.sender is not owner");

        TreeData storage treeData = treeDatas[treeId];
        require(timestamp > treeData.timestamp, "timestamp must increase");
        require(size >= treeData.size, "tree size can't decrease");

        if (size == treeData.size) {
            require(
                consistencyProof.length == 0,
                "empty proof is expected when tree size stays the same"
            );
        } else {
            // size > treeData.size
            if (treeData.size == 0) {
                require(
                    consistencyProof.length == 1,
                    "for an empty tree, proofs must have length=1"
                );
                treeData.root = consistencyProof[0];
            } else {
                treeData.root = consumeConsistencyProof(
                    treeData.root,
                    treeData.size,
                    consistencyProof,
                    size
                );
            }
            treeData.size = size;
        }
        treeData.timestamp = timestamp;
        treeData.signature = signature;

        emit Updated(treeId, treeData, consistencyProof);
        return treeData;
    }

    /// Consumes a consistency proof, verifying that it corresponds with the
    ///   given sizes and oldRoot, and returning the newRoot.
    /// @param oldRoot the expected old root of the tree.
    /// @param oldSize size of the tree before applying the consistency proof.
    /// @param proof list of hashes comprising the consistency proof.
    /// @param newSize size of the tree after applying the consistency proof.
    /// @dev consume a consistency proof for given tree sizes.
    /// @return root of the tree after applying consistency proof.
    function consumeConsistencyProof(
        bytes32 oldRoot,
        uint256 oldSize,
        bytes32[] calldata proof,
        uint256 newSize
    ) public pure returns (bytes32) {
        assert(0 < oldSize);
        assert(oldSize < newSize);
        // Based on oldSize, determine seed.
        uint256 seedLayer = getTrailingZeros(oldSize);
        // If oldSize was a power of 2, it is the seed.
        bool seedIsOldRoot = oldSize == (1 << seedLayer);
        (uint256 inner, uint256 border) = decompInclProof(oldSize - 1, newSize);
        uint256 expectedProofLen =
            inner + border -
                seedLayer +
                (seedIsOldRoot ? 0 : 1);
        require(
            proof.length == expectedProofLen,
            "got incorrect proof length based on oldSize and newSize"
        );
        bytes32 newSeed = oldRoot;
        if (!seedIsOldRoot) {
          newSeed = proof[0];
          proof = proof[1:];
        }
        // Track a second seed for reconstructing oldRoot.
        bytes32 oldSeed = newSeed;
        // Using oldSize, follow merge path to merge proof into both seeds, to
        //   reconstruct the old and new tree hash.
        uint256 seedIdx = (oldSize - 1) >> seedLayer;
        for (uint256 pIdx = 0; pIdx < proof.length; pIdx++) {
            // Right merges are only viable if the seed's idx is even
            // AND the seed hasn't reached the right border yet.
            if (seedIdx & 1 == 0 && (pIdx + seedLayer) < inner) {
                // Right merge, only merge into newSeed.
                newSeed = hashToParent(newSeed, proof[pIdx]);
            } else {
                // Left merge, merge into both newSeed and oldSeed.
                newSeed = hashToParent(proof[pIdx], newSeed);
                oldSeed = hashToParent(proof[pIdx], oldSeed);
            }
            // Note: seedIdx is only valid in the "inner" portion of the proof.
            seedIdx >>= 1;
        }
        require(
            oldSeed == oldRoot,
            "consistency proof did not correspond with oldRoot"
        );
        return newSeed;
    }

    function getTrailingZeros(uint256 n) public pure returns (uint256 count) {
        if (n != 0) {
            while (n & 1 == 0) {
                n >>= 1;
                count++;
            }
        }
    }

    /// Returns the min number of bits to represent n.
    ///   Returns 0 for 0, 1 for 1, 2 for 2, 6 for 35, etc.
    function minLen(uint256 n) public pure returns (uint256) {
        if (n == 0) {
            return 0;
        }
        return mostSignificantBit(n) + 1;
    }

    /// Return expected length of incl. proof for given leaf index and tree size
    ///   broken down into inner and border portions.
    function decompInclProof(uint256 index, uint256 size)
        public
        pure
        returns (uint256 inner, uint256 border)
    {
        // Calculate difference between incl. proofs of leaves |index| and
        // the last leaf (|size - 1|). This makes up the "lower" proof portion.
        inner = minLen(index ^ (size - 1));
        // Left merges above inner on the merge path make up the "upper" proof
        // portion.
        border = countSetBits(index >> inner);
    }
}

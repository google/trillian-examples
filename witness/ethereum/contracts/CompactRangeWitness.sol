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
/// @title Witness tracking external Merkle Tree via compact ranges.
contract CompactRangeWitness is WitnessUtils {
    /// @dev struct to encapsulate data for a specific tree.
    struct TreeData {
        bytes32[] compactRange;
        bytes32 signature;
        uint256 size;
        uint256 timestamp;
    }

    /// @dev emits information related to an update method call.
    event Updated(
        bytes32 treeId,
        bytes32 root,
        uint256 treeSize,
        uint256 timestamp,
        bytes32[] appendedCompactRange,
        bytes32 signature
    );

    address public owner;
    /// @dev mapping from treeId to that tree's data.
    mapping(bytes32 => TreeData) public treeDatas;

    constructor() {
        owner = msg.sender;
    }

    /// @dev getter for root for a specific treeId.
    function getRootForTreeId(bytes32 treeId) public view returns (bytes32) {
        return calcMerkleRoot(treeDatas[treeId].compactRange);
    }

    /// @dev getter for entire treeData value of a specific treeId.
    function getTreeDataById(bytes32 treeId)
        public
        view
        returns (TreeData memory)
    {
        return treeDatas[treeId];
    }

    /// Handles an update for the witness for a specific treeId.
    /// @param treeId the id of the tree the update is for.
    /// @param rightRange the compact range covering the new leaves. This is
    ///    only the range from [treeSize,rightRangeEnd).
    /// @param size the size of the tree after adding rightRange.
    ///   Corresponds to end index of the right range.
    /// @param timestamp integer timestamp of the update.
    /// @param signature raw signature over the marshalled bytes of new root.
    /// @dev merge rightRange into compactRange.
    /// @return newRoot the new merkle root.
    function update(
        bytes32 treeId,
        bytes32[] memory rightRange,
        uint256 size,
        uint256 timestamp,
        bytes32 signature
    ) external returns (bytes32 newRoot) {
        require(owner == msg.sender, "msg.sender is not owner");

        TreeData storage treeData = treeDatas[treeId];
        require(timestamp > treeData.timestamp, "timestamp may only increase");
        require(size >= treeData.size, "tree size must not decrease");

        if (size > treeData.size) {
            treeData.compactRange = mergeCompactRanges(
                /* leftRangeStart= */
                0,
                treeData.compactRange,
                treeData.size,
                rightRange,
                size
            );
            treeData.size = size;
        } else {
            // size == treeData.size
            require(
                rightRange.length == 0,
                "when tree size stays the same, rightRange must be empty"
            );
        }
        treeData.signature = signature;
        treeData.timestamp = timestamp;

        newRoot = calcMerkleRoot(treeData.compactRange);
        emit Updated(
            treeId,
            newRoot,
            size,
            timestamp,
            rightRange,
            treeData.signature
        );
    }

    /// @dev pure function for calculating a merkle root from a compact range
    function calcMerkleRoot(bytes32[] memory range)
        public
        pure
        returns (bytes32 seed)
    {
        // Start with the rightmost leaf and repeatedly fold inwards
        seed = range.length > 0 ? range[range.length - 1] : sha256("");
        for (uint256 i = 2; i <= range.length; i++) {
            seed = hashToParent(range[range.length - i], seed);
        }
    }

    /// @dev pure method to merge two compact ranges
    /// @return the merged compact range
    function mergeCompactRanges(
        uint256 leftRangeStart,
        bytes32[] memory leftRange,
        uint256 leftRangeEnd,
        bytes32[] memory rightRange,
        uint256 rightRangeEnd
    ) public pure returns (bytes32[] memory) {
        require(
            leftRange.length ==
                getExpectedNumHashesInCompactRange(
                    leftRangeStart,
                    leftRangeEnd
                ),
            "left range bounds and number of hashes don't correspond"
        );
        require(
            rightRange.length ==
                getExpectedNumHashesInCompactRange(leftRangeEnd, rightRangeEnd),
            "right range bounds and number of hashes don't correspond"
        );
        require(rightRange.length > 0, "right range must be non-empty");

        // Node at leftRangeEnd (ie. leftmost node in rightRange) becomes the
        // seed; leftRangeEnd's bits from lsb to msb indicate the merge path
        // from leaf to root; merge by folding left and right borders into seed,
        // following the merge path.:
        //   - 0 means fold with right border (if possible)
        //   - 1 means fold with left border (if possible)
        // Index of the bit in leftRangeEnd corresponds to mergeHeight.
        uint256 leftIdx = leftRange.length;
        uint256 rightIdx = 0;
        bytes32 seed = rightRange[rightIdx++];

        for (uint256 mergeHeight = 0; mergeHeight < 256; mergeHeight++) {
            uint256 numNodesCoveredBySeed = 1 << mergeHeight;
            // Floor div by (2 ** mergeHeight) to get seedIdx of a given height.
            uint256 seedIdx = leftRangeEnd >> mergeHeight;
            uint256 seedCoveredRangeStartIdx = seedIdx * numNodesCoveredBySeed;
            if ((seedIdx & 1) == 0) {
                // Right merge
                // Break if not enough nodes in remaining right range to viably
                // merge.
                if (
                    rightRangeEnd - seedCoveredRangeStartIdx <
                    2 * numNodesCoveredBySeed
                ) {
                    break;
                }
                // Skip all of the right merges that occur before a left merge,
                // since seed was initialized from the right border, so any
                // viable merges should already be folded in.
                if ((numNodesCoveredBySeed - 1) & leftRangeEnd == 0) {
                    continue;
                }
                seed = hashToParent(seed, rightRange[rightIdx++]);
            } else {
                // Left merge.
                // Break if not enough nodes in remaining left range to viably
                // merge.
                if (
                    (seedCoveredRangeStartIdx - leftRangeStart) <
                    numNodesCoveredBySeed
                ) {
                    break;
                }
                seed = hashToParent(leftRange[--leftIdx], seed);
            }
        }

        bytes32[] memory merged =
            concatRanges(leftRange, leftIdx, seed, rightRange, rightIdx);
        uint256 expectedNumHashes =
            getExpectedNumHashesInCompactRange(leftRangeStart, rightRangeEnd);
        assert(merged.length == expectedNumHashes);

        return merged;
    }

    /// @dev pure function to calculate size of a compact range from its bounds
    function getExpectedNumHashesInCompactRange(uint256 begin, uint256 end)
        public
        pure
        returns (uint256)
    {
        if (begin == 0) {
            return countSetBits(end);
        }
        // Decompose range into left and right subranges, each composed of
        // complement nodes of their outer border-node's merge path, where
        // they are diverged.
        uint256 xbegin = begin - 1;
        uint256 d = mostSignificantBit(xbegin ^ end);
        uint256 mask = (1 << d) - 1;

        uint256 left = (~xbegin) & mask;
        uint256 right = end & mask;

        return countSetBits(left) + countSetBits(right);
    }

    /// @dev merge a left and right compact range
    function concatRanges(
        bytes32[] memory left,
        uint256 leftEndIdx,
        bytes32 seed,
        bytes32[] memory right,
        uint256 rightStartIdx
    ) public pure returns (bytes32[] memory merged) {
        merged = new bytes32[](leftEndIdx + 1 + right.length - rightStartIdx);
        for (uint256 i = 0; i < leftEndIdx; i++) {
            merged[i] = left[i];
        }
        merged[leftEndIdx] = seed;
        for (uint256 i = rightStartIdx; i < right.length; i++) {
            merged[leftEndIdx + 1 + i - rightStartIdx] = right[i];
        }
        return merged;
    }
}

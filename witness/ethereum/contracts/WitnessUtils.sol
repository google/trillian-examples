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

/// @author sinasabet@google.com
/// @title Util class for use by full-fledged witnesses.
contract WitnessUtils {
    /// @dev hash two child nodes into a parent hash
    function hashToParent(bytes32 left, bytes32 right)
        public
        pure
        returns (bytes32)
    {
        return sha256(abi.encodePacked(hex"01", left, right));
    }

    /// Pure function to return the index of the most significant bit.
    ///   Returns 0 for 0, 0 for 1, 1 for 2, 5 for 35, etc.
    /// @dev returns index of the most significant bit
    function mostSignificantBit(uint256 n) public pure returns (uint256 msb) {
        // Search by repeatedly dividing the bitrange in half to find the msb.
        uint256 bitRangeSize = 256;
        while (n > 0 && bitRangeSize > 1) {
            bitRangeSize /= 2;
            uint256 t = n >> bitRangeSize;
            if (t != 0) {
                msb += bitRangeSize;
                n = t;
            }
        }
    }

    /// @dev returns number of 1 bits in binary representation of an int
    function countSetBits(uint256 n) public pure returns (uint256 count) {
        // Use Brian Kernighan’s algorithm to find number of set bits.
        while (n != 0) {
            count++;
            n &= (n - 1);
        }
    }
}

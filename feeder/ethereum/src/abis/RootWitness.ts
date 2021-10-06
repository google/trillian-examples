export const abi = [
  { inputs: [], stateMutability: "nonpayable", type: "constructor" },
  {
    anonymous: false,
    inputs: [
      {
        indexed: false,
        internalType: "bytes32",
        name: "treeId",
        type: "bytes32",
      },
      {
        components: [
          { internalType: "bytes32", name: "root", type: "bytes32" },
          { internalType: "uint256", name: "size", type: "uint256" },
          { internalType: "uint256", name: "timestamp", type: "uint256" },
          { internalType: "bytes32", name: "signature", type: "bytes32" },
        ],
        indexed: false,
        internalType: "struct RootWitness.TreeData",
        name: "newTreeData",
        type: "tuple",
      },
      {
        indexed: false,
        internalType: "bytes32[]",
        name: "consistencyProof",
        type: "bytes32[]",
      },
    ],
    name: "Updated",
    type: "event",
  },
  {
    inputs: [
      {
        components: [
          { internalType: "bytes32", name: "treeId", type: "bytes32" },
          { internalType: "bytes32[]", name: "proof", type: "bytes32[]" },
          { internalType: "uint256", name: "size", type: "uint256" },
          { internalType: "uint256", name: "timestamp", type: "uint256" },
          { internalType: "bytes32", name: "signature", type: "bytes32" },
        ],
        internalType: "struct RootWitness.UpdateData[]",
        name: "updateDatas",
        type: "tuple[]",
      },
    ],
    name: "batchUpdate",
    outputs: [
      {
        components: [
          { internalType: "bytes32", name: "root", type: "bytes32" },
          { internalType: "uint256", name: "size", type: "uint256" },
          { internalType: "uint256", name: "timestamp", type: "uint256" },
          { internalType: "bytes32", name: "signature", type: "bytes32" },
        ],
        internalType: "struct RootWitness.TreeData[]",
        name: "",
        type: "tuple[]",
      },
    ],
    stateMutability: "nonpayable",
    type: "function",
  },
  {
    inputs: [
      { internalType: "bytes32", name: "oldRoot", type: "bytes32" },
      { internalType: "uint256", name: "oldSize", type: "uint256" },
      { internalType: "bytes32[]", name: "proof", type: "bytes32[]" },
      { internalType: "uint256", name: "newSize", type: "uint256" },
    ],
    name: "consumeConsistencyProof",
    outputs: [{ internalType: "bytes32", name: "", type: "bytes32" }],
    stateMutability: "pure",
    type: "function",
  },
  {
    inputs: [{ internalType: "uint256", name: "n", type: "uint256" }],
    name: "countSetBits",
    outputs: [{ internalType: "uint256", name: "count", type: "uint256" }],
    stateMutability: "pure",
    type: "function",
  },
  {
    inputs: [
      { internalType: "uint256", name: "index", type: "uint256" },
      { internalType: "uint256", name: "size", type: "uint256" },
    ],
    name: "decompInclProof",
    outputs: [
      { internalType: "uint256", name: "inner", type: "uint256" },
      { internalType: "uint256", name: "border", type: "uint256" },
    ],
    stateMutability: "pure",
    type: "function",
  },
  {
    inputs: [{ internalType: "uint256", name: "n", type: "uint256" }],
    name: "getTrailingZeros",
    outputs: [{ internalType: "uint256", name: "count", type: "uint256" }],
    stateMutability: "pure",
    type: "function",
  },
  {
    inputs: [{ internalType: "bytes32", name: "treeId", type: "bytes32" }],
    name: "getTreeDataById",
    outputs: [
      {
        components: [
          { internalType: "bytes32", name: "root", type: "bytes32" },
          { internalType: "uint256", name: "size", type: "uint256" },
          { internalType: "uint256", name: "timestamp", type: "uint256" },
          { internalType: "bytes32", name: "signature", type: "bytes32" },
        ],
        internalType: "struct RootWitness.TreeData",
        name: "",
        type: "tuple",
      },
    ],
    stateMutability: "view",
    type: "function",
  },
  {
    inputs: [
      { internalType: "bytes32", name: "left", type: "bytes32" },
      { internalType: "bytes32", name: "right", type: "bytes32" },
    ],
    name: "hashToParent",
    outputs: [{ internalType: "bytes32", name: "", type: "bytes32" }],
    stateMutability: "pure",
    type: "function",
  },
  {
    inputs: [{ internalType: "uint256", name: "n", type: "uint256" }],
    name: "minLen",
    outputs: [{ internalType: "uint256", name: "", type: "uint256" }],
    stateMutability: "pure",
    type: "function",
  },
  {
    inputs: [{ internalType: "uint256", name: "n", type: "uint256" }],
    name: "mostSignificantBit",
    outputs: [{ internalType: "uint256", name: "msb", type: "uint256" }],
    stateMutability: "pure",
    type: "function",
  },
  {
    inputs: [],
    name: "owner",
    outputs: [{ internalType: "address", name: "", type: "address" }],
    stateMutability: "view",
    type: "function",
  },
  {
    inputs: [{ internalType: "bytes32", name: "", type: "bytes32" }],
    name: "treeDatas",
    outputs: [
      { internalType: "bytes32", name: "root", type: "bytes32" },
      { internalType: "uint256", name: "size", type: "uint256" },
      { internalType: "uint256", name: "timestamp", type: "uint256" },
      { internalType: "bytes32", name: "signature", type: "bytes32" },
    ],
    stateMutability: "view",
    type: "function",
  },
  {
    inputs: [
      { internalType: "bytes32", name: "treeId", type: "bytes32" },
      {
        internalType: "bytes32[]",
        name: "consistencyProof",
        type: "bytes32[]",
      },
      { internalType: "uint256", name: "size", type: "uint256" },
      { internalType: "uint256", name: "timestamp", type: "uint256" },
      { internalType: "bytes32", name: "signature", type: "bytes32" },
    ],
    name: "update",
    outputs: [
      {
        components: [
          { internalType: "bytes32", name: "root", type: "bytes32" },
          { internalType: "uint256", name: "size", type: "uint256" },
          { internalType: "uint256", name: "timestamp", type: "uint256" },
          { internalType: "bytes32", name: "signature", type: "bytes32" },
        ],
        internalType: "struct RootWitness.TreeData",
        name: "",
        type: "tuple",
      },
    ],
    stateMutability: "nonpayable",
    type: "function",
  },
];

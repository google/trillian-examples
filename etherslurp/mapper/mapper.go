// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mapper

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/golang/glog"
	"github.com/google/trillian"
)

const (
	oneEther int64 = 1000000000000000000
)

type Mapper struct {
	logID, mapID int64
	tlog         trillian.TrillianLogClient
	tmap         trillian.TrillianMapClient

	unsortedBlocks chan *types.Block
	sortedBlocks   chan *types.Block
}

func New(tl trillian.TrillianLogClient, logID int64, tm trillian.TrillianMapClient, mapID int64) *Mapper {
	return &Mapper{
		logID: logID,
		mapID: mapID,
		tlog:  tl,
		tmap:  tm,

		unsortedBlocks: make(chan *types.Block, 100000),
		sortedBlocks:   make(chan *types.Block, 100000),
	}
}

func (m *Mapper) fetchBlocks(ctx context.Context, from int64) {
	ticker := time.NewTicker(time.Second)

nextAttempt:
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		num := int64(200)
		leaves := make([]int64, num)
		for i := int64(0); i < num; i++ {
			leaves[i] = from + i
		}

		entries, err := m.tlog.GetLeavesByIndex(ctx, &trillian.GetLeavesByIndexRequest{LogId: m.logID, LeafIndex: leaves})
		if err != nil {
			glog.Errorf("Failed to get %d leaves starting at index %d: %v", num, from, err)
			continue nextAttempt
		}

		for _, l := range entries.Leaves {
			block := &types.Block{}
			if err := rlp.DecodeBytes(l.LeafValue, block); err != nil {
				glog.Errorf("Failed to decode block from log at index %d: %v", l.LeafIndex, err)
				continue nextAttempt
			}
			m.unsortedBlocks <- block
		}

		from += num
	}
}

func (m *Mapper) pipelineBlocks(ctx context.Context, from int64) {
	ticker := time.NewTicker(time.Second)
	blocksByNumber := make(map[int64]*types.Block)

	go m.fetchBlocks(ctx, from)

nextAttempt:
	for {
		select {
		case <-ctx.Done():
			for range m.unsortedBlocks {
			}
			return
		case b := <-m.unsortedBlocks:
			blocksByNumber[b.Number().Int64()] = b
		case <-ticker.C:
		}

		// try to sort some blocks:
		for {
			b, found := blocksByNumber[from]
			if !found {
				continue nextAttempt
			}
			m.sortedBlocks <- b
			from++
		}

	}
}

func isProtectedV(V *big.Int) bool {
	if V.BitLen() <= 8 {
		v := V.Uint64()
		return v != 27 && v != 28
	}
	// anything not 27 or 28 are considered unprotected
	return true
}

// deriveChainId derives the chain id from the given v parameter
func deriveChainId(v *big.Int) *big.Int {
	if v.BitLen() <= 64 {
		v := v.Uint64()
		if v == 27 || v == 28 {
			return new(big.Int)
		}
		return new(big.Int).SetUint64((v - 35) / 2)
	}
	v = new(big.Int).Sub(v, big.NewInt(35))
	return v.Div(v, big.NewInt(2))
}

// deriveSigner makes a *best* guess about which signer to use.
func deriveSigner(V *big.Int) types.Signer {
	if V.Sign() != 0 && isProtectedV(V) {
		return types.NewEIP155Signer(deriveChainId(V))
	} else {
		return types.HomesteadSigner{}
	}
}

func fmtAddress(a []byte) string {
	return fmt.Sprintf("%x", a[:])
}

func index(a []byte) string {
	r := sha256.Sum256(a)
	return string(r[:])
}

func (m *Mapper) mapTransactionsFrom(ctx context.Context, b *types.Block) error {
	numTX := len(b.Transactions())
	if numTX == 0 {
		return nil
	}
	glog.Infof("Mapping %d transactions from block @ %v", len(b.Transactions()), b.Number())
	glog.V(1).Infof("Block: %v", b.String())

	deltas := make(map[string]*big.Int)

	// Add miner credit
	minerIndex := index(b.Coinbase().Bytes())
	credit := big.NewInt(int64(5) * int64(1+len(b.Uncles())/32))
	credit.Mul(credit, big.NewInt(oneEther))
	deltas[minerIndex] = credit

	for i, tx := range b.Transactions() {
		v, _, _ := tx.RawSignatureValues()
		if v == nil {
			return fmt.Errorf("nil signature on tx@%d@%v", i, b.Number())
		}
		signer := deriveSigner(v)
		from, err := types.Sender(signer, tx)
		if err != nil {
			return fmt.Errorf("unable to derive sender on tx@%d@%v", i, b.Number())
		}
		to := tx.To()
		if to == nil {
			glog.Infof("Ignoring start-contract TX with nil To: address %d@%v", i, b.Number())
			continue
		}

		rIndex := index(to.Bytes())
		rBal, ok := deltas[rIndex]
		if !ok {
			rBal = big.NewInt(0)
		}
		rBal.Add(rBal, tx.Value())
		deltas[rIndex] = rBal

		sIndex := index(from.Bytes())
		sBal, ok := deltas[sIndex]
		if !ok {
			sBal = big.NewInt(0)
		}
		sBal.Sub(sBal, tx.Cost())
		deltas[sIndex] = sBal

		{
			// only using floats for printing, map should use the fixed point representation!
			sender := fmtAddress(from.Bytes())
			recipient := fmtAddress(to.Bytes())
			amount := float64(tx.Value().Int64()) / float64(oneEther)
			cost := float64(tx.Cost().Int64()) / float64(oneEther)
			glog.Infof("Ξ%f from %s... (%x...) -> %s... (%x...), costing Ξ%f", amount, sender[:5], sIndex[:5], recipient[:5], rIndex[:5], cost)
		}
	}
	glog.V(1).Infof("Have %d deltas", len(deltas))
	if len(deltas) == 0 {
		return nil
	}

	getRequest := &trillian.GetMapLeavesRequest{
		MapId: m.mapID,
	}
	for k := range deltas {
		getRequest.Index = append(getRequest.Index, []byte(k))
	}

	glog.V(1).Info("Get map leaves...")
	get, err := m.tmap.GetLeaves(ctx, getRequest)
	if err != nil {
		return fmt.Errorf("failed to get current balances: %v", err)
	}
	glog.V(1).Infof("Got %d map leaves.", len(get.MapLeafInclusion))

	if ld, ll := len(deltas), len(get.MapLeafInclusion); ld != ll {
		glog.Exitf("Got %d leaves, expected %d", ll, ld)
	}

	setRequest := &trillian.SetMapLeavesRequest{
		MapId:  m.mapID,
		Leaves: make([]*trillian.MapLeaf, 0),
	}

	for _, l := range get.MapLeafInclusion {
		bal := big.NewInt(0)
		if len(l.Leaf.LeafValue) > 0 {
			var ok bool
			bal, ok = bal.SetString(string(l.Leaf.LeafValue), 10)
			if !ok {
				glog.Warningf("Leaf value for %x... (%s) is corrupt, resetting to zero balance", l.Leaf.Index[:5], string(l.Leaf.LeafValue))
				bal = big.NewInt(0)
			}
		}
		k := string(l.Leaf.Index)
		d, ok := deltas[k]
		if !ok {
			glog.Warning("No delta for leaf index %x", l.Leaf.Index)
			continue
		}
		delete(deltas, k)
		glog.V(1).Infof("index %x... had: Ξ%f", l.Leaf.Index[:5], (float64(bal.Int64()) / float64(oneEther)))
		bal.Add(bal, d)
		l.Leaf.LeafValue = []byte(bal.String())
		setRequest.Leaves = append(setRequest.Leaves, l.Leaf)
		glog.Infof("index %x... now has: Ξ%f", l.Leaf.Index[:5], (float64(bal.Int64()) / float64(oneEther)))
	}

	if len(deltas) != 0 {
		glog.Exitf("Arg, didn't use all deltas, still have:\n%+v", deltas)
	}

	glog.V(1).Infof("Setting %d map leaves.", len(setRequest.Leaves))
	_, err = m.tmap.SetLeaves(ctx, setRequest)
	if err != nil {
		return fmt.Errorf("failed to update balances: %v", err)
	}

	return nil
}

func (m *Mapper) Map(ctx context.Context) {
	from := int64(0)
	go m.pipelineBlocks(ctx, from)

	for {
		select {
		case <-ctx.Done():
			return
		case nextBlock := <-m.sortedBlocks:
			if nextBlock.Number().Int64() != from {
				glog.Exitf("Got unexpected block number %s, wanted %d", nextBlock.Number(), from)
			}
			// TODO(al): batching...
			if err := m.mapTransactionsFrom(ctx, nextBlock); err != nil {
				glog.Exitf("Couldn't map transactions from block %v", err)
			}
			from++
		}
	}
}

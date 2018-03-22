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

package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"math/big"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/types"
)

const page = `
<head>
  <title>EtherSlurp</title>
</head>
<body>
	<h2>EtherSlurp</h2>
	<form method="get">
		AccountID: <input type="text" name="account"/><input type="submit" value="Go!"/>
	</form>
	{{if .ErrorText}}
		<font color="darkred">{{.ErrorText}}</font>
	{{else if .AccountID}}
	Account <i>{{.AccountID}}</i><br/>
	Balance <i>{{.Amount}}</i><br/>
	As at block <i>XXX</i><br/>
	<br/>
	<br/>
	Inclusion Proof is
	{{if .ProofValid}}
	  <font color="green">{{.ProofDesc}}</font>
  {{else}}
	  <font color="red">{{.ProofDesc}}</font>
	{{end}}
	<br/>
	<br/>
	SMR:<br/>
	<pre>{{.SMR}}</pre>
	</br>
	</br>
	InclusionProof:<br/>
	<pre>{{.Proof}}</pre>
	<br/>
	{{end}}

	<br/>
	<br/>
	<br/>
	<font color="grey">Looking for accountIDs? Snarf some from <a href="https://rinkeby.etherscan.io/txs?p=10000">here</a>.</font>
</body>
`
const (
	oneEther int64 = 1000000000000000000
)

var oneEtherRatio = big.NewFloat(float64(1) / float64(oneEther))

// New creates a new UI.
func New(tmc trillian.TrillianMapClient, mapID int64) *UI {
	return &UI{
		mapID: mapID,
		tmc:   tmc,
		tmpl:  template.Must(template.New("root").Parse(page)),
	}
}

// UI encapsulates data related to serving the web ui for the application.
type UI struct {
	mapID int64
	tmc   trillian.TrillianMapClient
	tmpl  *template.Template
}

const keyAccount = "account"

type accountInfo struct {
	AccountID  string
	Amount     string
	ErrorText  string
	ProofValid bool
	ProofDesc  string
	Proof      string
	SMR        string
}

func index(a []byte) []byte {
	r := sha256.Sum256(a)
	return r[:]
}

func (ui *UI) getLeaf(ctx context.Context, ac string) (*trillian.MapLeafInclusion, *trillian.SignedMapRoot, error) {
	ac = strings.TrimPrefix(ac, "0x")
	acBytes, err := hex.DecodeString(ac)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't decode accountID hex string: %v", err)
	}

	getRequest := &trillian.GetMapLeavesRequest{
		MapId: ui.mapID,
		Index: [][]byte{index(acBytes)},
	}

	glog.Info("Get map leaves...")
	get, err := ui.tmc.GetLeaves(ctx, getRequest)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get current balance for ac %v: %v", ac, err)
	}
	glog.Infof("Got %d map leaves.", len(get.MapLeafInclusion))
	return get.MapLeafInclusion[0], get.MapRoot, nil
}

func ethBalance(b *big.Int) string {
	a := &big.Float{}
	a.SetInt(b)
	a = a.Mul(a, oneEtherRatio)
	return fmt.Sprintf("Ξ%s", a.String())
}

func jsonOrErr(a interface{}) string {
	r, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(r)
}

func (ui *UI) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	acString := req.FormValue(keyAccount)

	var ac accountInfo
	if acString != "" {
		ac.AccountID = acString
		leafInc, smr, err := ui.getLeaf(req.Context(), acString)
		if err != nil {
			ac.ErrorText = err.Error()
		} else {
			if len(leafInc.Leaf.LeafValue) == 0 {
				ac.ErrorText = fmt.Sprintf("Account %s is unknown", acString)
			} else {
				bal := big.NewInt(0)
				var ok bool
				bal, ok = bal.SetString(string(leafInc.Leaf.LeafValue), 10)
				if !ok {
					ac.ErrorText = fmt.Sprintf("Couldn't parse account balance %v", string(leafInc.Leaf.LeafValue))
				} else {
					ac.Amount = ethBalance(bal)
					var root types.MapRootV1
					if err := root.UnmarshalBinary(smr.MapRoot); err != nil {
						ac.ProofValid = false
						ac.ProofDesc = fmt.Sprintf("ERROR: %s", err)
					} else {
						err := merkle.VerifyMapInclusionProof(ui.mapID, leafInc.Leaf.Index, leafInc.Leaf.LeafValue, root.RootHash, leafInc.Inclusion, maphasher.Default)
						if err != nil {
							ac.ProofValid = false
							ac.ProofDesc = fmt.Sprintf("INVALID: %s", err)
						} else {
							ac.ProofValid = true
							ac.ProofDesc = "VALID"
						}
					}
					ac.Proof = jsonOrErr(leafInc.Inclusion)
					ac.SMR = jsonOrErr(smr)
				}
			}
		}
	}
	if err := ui.tmpl.Execute(w, ac); err != nil {
		glog.Errorf("Failed to write template: %v", err)
	}

}

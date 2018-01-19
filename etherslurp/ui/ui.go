package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"math/big"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian"
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
	Account <b>{{.AccountID}}</b> has <b>{{.Amount}}<b/>
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

func New(tmc trillian.TrillianMapClient, mapID int64) *UI {
	return &UI{
		mapID: mapID,
		tmc:   tmc,
		tmpl:  template.Must(template.New("root").Parse(page)),
	}
}

type UI struct {
	mapID int64
	tmc   trillian.TrillianMapClient
	tmpl  *template.Template
}

const keyAccount = "account"

type accountInfo struct {
	AccountID string
	Amount    string
	ErrorText string
}

func index(a []byte) []byte {
	r := sha256.Sum256(a)
	return r[:]
}

func (ui *UI) getLeaf(ctx context.Context, ac string) (*trillian.MapLeaf, error) {
	if strings.HasPrefix(ac, "0x") {
		ac = ac[2:]
	}
	acBytes, err := hex.DecodeString(ac)
	if err != nil {
		return nil, fmt.Errorf("couldn't decode accountID hex string: %v", err)
	}

	getRequest := &trillian.GetMapLeavesRequest{
		MapId: ui.mapID,
		Index: [][]byte{index(acBytes)},
	}

	glog.Info("Get map leaves...")
	get, err := ui.tmc.GetLeaves(ctx, getRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to get current balance for ac %v: %v", ac, err)
	}
	glog.Infof("Got %d map leaves.", len(get.MapLeafInclusion))
	return get.MapLeafInclusion[0].Leaf, nil
}

func ethBalance(b *big.Int) string {
	a := &big.Float{}
	a.SetInt(b)
	a = a.Mul(a, oneEtherRatio)
	return fmt.Sprintf("Ξ%s", a.String())
}

func (ui *UI) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	acString := req.FormValue(keyAccount)

	var ac accountInfo
	if acString != "" {
		ac.AccountID = acString
		leaf, err := ui.getLeaf(req.Context(), acString)
		if err != nil {
			ac.ErrorText = err.Error()
		}
		if len(leaf.LeafValue) == 0 {
			ac.ErrorText = fmt.Sprintf("Account %s is unknown", acString)
		} else {
			bal := big.NewInt(0)
			var ok bool
			bal, ok = bal.SetString(string(leaf.LeafValue), 10)
			if !ok {
				ac.ErrorText = fmt.Sprintf("Couldn't parse account balance %v", string(leaf.LeafValue))
			} else {
				ac.Amount = ethBalance(bal)
			}
		}
	}
	if err := ui.tmpl.Execute(w, ac); err != nil {
		glog.Errorf("Failed to write template: %v", err)
	}

}

func (ui *UI) sendSearchForm(w http.ResponseWriter) {
}

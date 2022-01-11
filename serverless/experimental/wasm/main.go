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

//go:build wasm
// +build wasm

// Package main provides a series of entrypoints for using a serverless log from
// JavaScript in a browser.
//
// See the accompanying README for details on how to spin this up in a browser.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall/js"
	"time"

	"github.com/google/trillian-examples/serverless/api"
	"github.com/google/trillian-examples/serverless/api/layout"
	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/internal/storage"
	"github.com/google/trillian-examples/serverless/internal/storage/webstorage"
	"github.com/google/trillian-examples/serverless/pkg/log"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"

	logfmt "github.com/google/trillian-examples/formats/log"
)

const (
	logPrefix = "log"
	origin    = "WASM Log"
)

var (
	logStorage *webstorage.Storage
	logSig     note.Signer
	logVer     note.Verifier
)

func b64Sha(b []byte) string {
	h := sha256.Sum256(b)
	b64 := base64.StdEncoding.EncodeToString(h[:])
	return b64
}

const caret = "<blink>▒</blink>"

func appendToElement(e, s string) {
	c := js.Global().Get(e)
	o := strings.TrimSuffix(c.Get("innerHTML").String(), caret)
	c.Set("innerHTML", fmt.Sprintf("%s%s</br>%s", o, s, caret))
	c.Set("scrollTop", c.Get("scrollHeight"))
}

func logMsg(s string) {
	appendToElement("logConsole", s)
}

func monMsg(s string) {
	appendToElement("monitorConsole", s)
}

func showCP(ctx context.Context, f client.Fetcher) {
	for {
		select {
		case <-time.Tick(time.Second):
		case <-ctx.Done():
			return
		}
		_, cp, err := client.FetchCheckpoint(ctx, f, logVer, origin)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				logMsg(err.Error())
				continue
			}
			cp = []byte("Checkpoint doesn't exist (yet) - queue, sequence, and integrate a leaf")
		}

		js.Global().Get("latestCP").Set("innerHTML", fmt.Sprintf("<pre>%s</pre>", cp))
	}
}

func integrate() js.Func {
	jsonFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 0 {
			return "Invalid number of arguments passed - want no args"
		}

		cp := &logfmt.Checkpoint{}
		cpRaw, err := webstorage.ReadCheckpoint(logPrefix)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				logMsg("No checkpoint found, log starting from scratch")
			} else {
				logMsg(fmt.Sprintf("Failed to read checkpoint: %v", err))
				panic(err)
			}
		} else {
			cp, _, _, err = logfmt.ParseCheckpoint(cpRaw, origin, logVer)
			if err != nil {
				logMsg(string(cpRaw))
				panic(err)
			}
		}

		ctx := context.Background()
		newCp, err := log.Integrate(ctx, *cp, logStorage, rfc6962.DefaultHasher)
		if err != nil {
			logMsg(fmt.Sprintf("Failed to integrate: %q", err))
			panic(err)
		}
		if newCp == nil {
			logMsg("Nothing to integrate")
			return nil
		}
		if err != nil {
			logMsg(err.Error())
			return nil
		}

		newCp.Origin = origin
		nRaw, err := note.Sign(&note.Note{Text: string(newCp.Marshal())}, logSig)
		if err != nil {
			logMsg(err.Error())
			return nil
		}
		if err := logStorage.WriteCheckpoint(ctx, nRaw); err != nil {
			logMsg(err.Error())
			return nil
		}

		logMsg("Integrate OK")
		return nil
	})
	return jsonFunc
}

func sequence() js.Func {
	jsonFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 0 {
			return "Invalid number of arguments passed - want no args"
		}

		pendingLeaves, err := logStorage.PendingKeys()
		if err != nil {
			logMsg(err.Error())
			return nil
		}
		for _, lk := range pendingLeaves {
			l, err := logStorage.Pending(lk)
			if err != nil {
				logMsg(err.Error())
				return nil
			}
			h := rfc6962.DefaultHasher.HashLeaf(l)
			isDupe := false
			seq, err := logStorage.Sequence(context.Background(), h, l)
			if err != nil {
				if !errors.Is(err, storage.ErrDupeLeaf) {
					logMsg(err.Error())
					return nil
				}
				isDupe = true
			}
			s := fmt.Sprintf("index %d: %q", seq, l)
			if isDupe {
				s += " (dupe)"
			}
			logMsg(s)
			if err := logStorage.DeletePending(lk); err != nil {
				logMsg(err.Error())
				return nil
			}

		}
		return nil
	})
	return jsonFunc
}

func queueLeaf() js.Func {
	jsonFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 1 {
			return "Invalid number of arguments passed - want 1 leaf"
		}
		leaf := args[0].String()
		fmt.Printf("leaf %q\n", leaf)

		if err := logStorage.Queue([]byte(leaf)); err != nil {
			return fmt.Sprintf("failed to queue leaf: %v", err)
		}
		logMsg(fmt.Sprintf("<i>Queued leaf %q</i>", leaf))
		return nil
	})
	return jsonFunc
}

func tileFetcher(treeSize uint64, f client.Fetcher) client.GetTileFunc {
	return func(ctx context.Context, l, i uint64) (*api.Tile, error) {
		dir, p := layout.TilePath("", l, i, treeSize)
		tRaw, err := f(ctx, path.Join(dir, p))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tile at level: %d, index: %d: %v", l, i, err)
		}
		t := &api.Tile{}
		if err := t.UnmarshalText(tRaw); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tile: %v", err)
		}
		return t, nil
	}
}

func monitor(ctx context.Context, f client.Fetcher) {
	cpCur := &logfmt.Checkpoint{}
	rf := &compact.RangeFactory{Hash: rfc6962.DefaultHasher.HashChildren}
	r := rf.NewEmptyRange(0)

monitorLoop:
	for {
		select {
		case <-time.Tick(time.Second):
		case <-ctx.Done():
			return
		}

		cp, _, err := client.FetchCheckpoint(ctx, f, logVer, origin)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				monMsg(err.Error())
			}
			// CP likely just doesn't exist yet - no sequence/integrate run has happened.
			continue
		}

		if cp.Size <= cpCur.Size {
			continue
		}
		monMsg(fmt.Sprintf("<invert>Saw new CP with size %d</invert>", cp.Size))

		rCP := rf.NewEmptyRange(cpCur.Size)
		for i := cpCur.Size; i < cp.Size; i++ {
			l, err := client.GetLeaf(ctx, f, i)
			if err != nil {
				monMsg(fmt.Sprintf("Failed to fetch leaf at %d: %v", i, err))
				break monitorLoop
			}
			monMsg(fmt.Sprintf(" + Leaf %d: <i>%q</i>", i, string(l)))

			if err := rCP.Append(rfc6962.DefaultHasher.HashLeaf(l), nil); err != nil {
				monMsg(fmt.Sprintf("Failed to update compact range for leaf at %d: %v", i, err))
				break monitorLoop
			}
		}
		if err := r.AppendRange(rCP, nil); err != nil {
			monMsg(fmt.Sprintf("Failed to merge compact ranges: %v", err))
			continue
		}
		root, err := r.GetRootHash(nil)
		if err != nil {
			monMsg(fmt.Sprintf("Failed to get root hash from compact range: %v", err))
			continue
		}
		if !bytes.Equal(cp.Hash, root) {
			monMsg(fmt.Sprintf("<b>Root hashes do not match: CP %x vs calculated %x</b>", cp.Hash, root))
			monMsg("<invert>Bailing</invert>")
			return
		}
		monMsg("New CP is consistent!")
		cpCur = cp
		monMsg("----------------------------------")
	}
}

func initKeys() {
	var s, v string
	var err error

	// Don't forget, this is a DEMO - don't do this for your production things:
	prevV := js.Global().Get("sessionStorage").Call("getItem", "log.pub")
	prevS := js.Global().Get("sessionStorage").Call("getItem", "log.sec")

	if !js.Null().Equal(prevV) {
		s = prevS.String()
		v = prevV.String()
		logMsg("Using previously generated keys")
	} else {
		s, v, err = note.GenerateKey(nil, "demo-log")
		if err != nil {
			panic(err)
		}
		js.Global().Get("sessionStorage").Call("setItem", "log.pub", v)
		js.Global().Get("sessionStorage").Call("setItem", "log.sec", s)
		logMsg("Generated new keys")
	}

	logSig, err = note.NewSigner(s)
	if err != nil {
		panic(err)
	}
	logVer, err = note.NewVerifier(v)
	if err != nil {
		panic(err)
	}
}

func main() {
	flag.Parse()
	flag.Set("logtostderr", "true")
	ctx := context.Background()

	fmt.Println("Serverless Web Assembly!")

	initKeys()

	js.Global().Set("queueLeaf", queueLeaf())
	js.Global().Set("sequence", sequence())
	js.Global().Set("integrate", integrate())

	cpRaw, err := webstorage.ReadCheckpoint(logPrefix)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			panic(err)
		}
		logStorage, err = webstorage.Create(logPrefix)
		if err != nil {
			panic(err)
		}
	} else {
		cp, _, _, err := logfmt.ParseCheckpoint(cpRaw, origin, logVer)
		if err != nil {
			logMsg(string(cpRaw))
			panic(err)
		}
		logStorage, err = webstorage.Load(logPrefix, cp.Size)
		if err != nil {
			panic(err)
		}
	}

	fetcher := func(ctx context.Context, path string) ([]byte, error) {
		v := js.Global().Get("sessionStorage").Call("getItem", filepath.Join(logPrefix, path))
		if js.Null().Equal(v) {
			return nil, os.ErrNotExist
		}
		return []byte(v.String()), nil

	}
	go showCP(ctx, fetcher)
	go monitor(ctx, fetcher)
	<-make(chan bool)
}

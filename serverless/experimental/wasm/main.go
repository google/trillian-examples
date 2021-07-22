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

// +build wasm

// Package main provides a series of entrypoints for using a serverless log from
// JavaScript in a browser.
//
// See the accompanying README for details on how to spin this up in a browser.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"syscall/js"
	"time"

	"github.com/google/trillian-examples/serverless/client"
	"github.com/google/trillian-examples/serverless/internal/log"
	"github.com/google/trillian-examples/serverless/internal/storage"
	"github.com/google/trillian-examples/serverless/internal/storage/webstorage"
	"github.com/google/trillian/merkle/logverifier"
	"github.com/google/trillian/merkle/rfc6962"
	"golang.org/x/mod/sumdb/note"

	logfmt "github.com/google/trillian-examples/formats/log"
)

const (
	logPrefix = "log"
	ecosystem = "WASM Log v0"
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

func logMsg(s interface{}) {
	c := js.Global().Get("logConsole")
	o := c.Get("innerHTML")
	c.Set("innerHTML", fmt.Sprintf("%s\n%v", o, s))
	c.Set("scrollTop", c.Get("scrollHeight"))
}

func monMsg(s interface{}) {
	c := js.Global().Get("monitorConsole")
	o := c.Get("innerHTML")
	c.Set("innerHTML", fmt.Sprintf("%s\n%v", o, s))
	c.Set("scrollTop", c.Get("scrollHeight"))
}

func showCP(ctx context.Context, f client.Fetcher) {
	for {
		select {
		case <-time.Tick(time.Second):
		case <-ctx.Done():
			return
		}
		_, cp, err := client.FetchCheckpoint(ctx, f, logVer)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				logMsg(err)
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

		newCp, err := log.Integrate(context.Background(), logStorage, rfc6962.DefaultHasher)
		if err != nil {
			panic(fmt.Errorf("Failed to integrate: %q", err))
		}
		if newCp == nil {
			logMsg("Nothing to integrate")
			return nil
		}
		if err != nil {
			logMsg(err)
			return nil
		}

		newCp.Ecosystem = ecosystem
		nRaw, err := note.Sign(&note.Note{Text: string(newCp.Marshal())}, logSig)
		if err != nil {
			logMsg(err)
			return nil
		}
		if err := logStorage.WriteCheckpoint(nRaw); err != nil {
			logMsg(err)
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
			logMsg(err)
			return nil
		}
		for _, lk := range pendingLeaves {
			l, err := logStorage.Pending(lk)
			if err != nil {
				logMsg(err)
				return nil
			}
			h := rfc6962.DefaultHasher.HashLeaf(l)
			isDupe := false
			seq, err := logStorage.Sequence(h, l)
			if err != nil {
				if !errors.Is(err, storage.ErrDupeLeaf) {
					logMsg(err)
					return nil
				}
				isDupe = true
			}
			s := fmt.Sprintf("index %d: %v", seq, lk)
			if isDupe {
				s += " (dupe)"
			}
			logMsg(s)
			if err := logStorage.DeletePending(lk); err != nil {
				logMsg(err)
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
		return nil
	})
	return jsonFunc
}

func monitor(ctx context.Context, f client.Fetcher) {
	cpCur := &logfmt.Checkpoint{}
	logProofVerifier := logverifier.New(rfc6962.DefaultHasher)
	for {
		select {
		case <-time.Tick(time.Second):
		case <-ctx.Done():
			return
		}

		cp, _, err := client.FetchCheckpoint(ctx, f, logVer)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				monMsg(err)
			}
			// CP likely just doesn't exist yet - no sequence/integrate run has happened.
			continue
		}

		if cp.Size <= cpCur.Size {
			continue
		}
		monMsg("----------------------------------")
		monMsg(fmt.Sprintf("<invert>Saw new CP with size %d</invert>", cp.Size))

		if cpCur.Size > 0 {
			pb, err := client.NewProofBuilder(ctx, *cp, rfc6962.DefaultHasher.HashChildren, f)
			if err != nil {
				monMsg(err)
				continue
			}
			proof, err := pb.ConsistencyProof(ctx, cpCur.Size, cp.Size)
			if err != nil {
				monMsg(err)
				continue
			}
			if err := logProofVerifier.VerifyConsistencyProof(int64(cpCur.Size), int64(cp.Size), cpCur.Hash, cp.Hash, proof); err != nil {
				monMsg(err)
				continue
			}
			monMsg("Proof:")
			monMsg(logfmt.Proof(proof).Marshal())
			monMsg("New CP verified to be consistent")
		}
		cpCur = cp
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
		logStorage, err = webstorage.Create(logPrefix, []byte("empty"))
		if err != nil {
			panic(err)
		}
	} else {
		n, err := note.Open(cpRaw, note.VerifierList(logVer))
		if err != nil {
			logMsg(string(cpRaw))
			panic(err)
		}
		cp := &logfmt.Checkpoint{}
		_, err = cp.Unmarshal([]byte(n.Text))
		if err != nil {
			panic(err)
		}
		logStorage, err = webstorage.Load(logPrefix, cp)
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

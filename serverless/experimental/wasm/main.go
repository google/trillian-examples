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
	"syscall/js"
	"time"

	"github.com/google/trillian-examples/serverless/internal/log"
	"github.com/google/trillian-examples/serverless/internal/storage"
	"github.com/google/trillian-examples/serverless/internal/storage/webstorage"
	"github.com/google/trillian/merkle/rfc6962"

	logfmt "github.com/google/trillian-examples/formats/log"
)

const (
	logPrefix = "log"
	ecosystem = "WASM Log v0"
)

var (
	logStorage *webstorage.Storage
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

func showCP() {
	cp, _ := webstorage.ReadCheckpoint(logPrefix)
	js.Global().Get("latestCP").Set("innerHTML", fmt.Sprintf("<pre>%s</pre>", cp))
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
		cpRaw := newCp.Marshal()
		if err := logStorage.WriteCheckpoint(cpRaw); err != nil {
			logMsg(err)
			return nil
		}

		logMsg("Integrate OK")
		showCP()
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

func monitor() {
	cpCur := &logfmt.Checkpoint{}

	for {
		<-time.Tick(time.Second)
		cpRaw, _ := webstorage.ReadCheckpoint(logPrefix)
		cp := &logfmt.Checkpoint{}
		if _, err := cp.Unmarshal(cpRaw); err != nil {
			monMsg(fmt.Sprintf("Couldn't parse CP: %v", err))
			continue
		}

		if cp.Size > cpCur.Size {
			monMsg("----------------------------------")
			monMsg(fmt.Sprintf("Saw new CP:\n%s", string(cp.Marshal())))
			cpCur = cp
		}

	}
}

func main() {
	flag.Parse()
	flag.Set("logtostderr", "true")

	fmt.Println("Serverless Web Assembly!")
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
		cp := &logfmt.Checkpoint{}
		_, err := cp.Unmarshal(cpRaw)
		if err != nil {
			panic(err)
		}
		logStorage, err = webstorage.Load(logPrefix, cp)
		if err != nil {
			panic(err)
		}
	}
	showCP()
	go monitor()
	<-make(chan bool)
}

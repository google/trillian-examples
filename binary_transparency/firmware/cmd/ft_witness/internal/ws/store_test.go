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

package ws

import (
	"bytes"
	"fmt"
	"testing"
)

const (
	dbFp    = "/tmp/wdb.db"
	undefFp = ""
)

func TestRoundTrip(t *testing.T) {
	for _, test := range []struct {
		desc string
		data []byte
	}{
		{
			desc: "initial checkpoint test",
			data: []byte("some checkpoint"),
		}, {
			desc: "check over-write of checkpoint",
			data: []byte("some more checkpoint"),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {

			store, err := NewStorage(dbFp)
			if err != nil {
				t.Error("failed to create storage", err)
			}
			want := test.data
			if err := store.StoreCP(test.data); err != nil {
				t.Error("failed to store into Witness Store", err)
			}
			got, err := store.RetrieveCP()
			if err != nil {
				t.Error("failed to retrieve from Witness Store", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("got '%s' want '%s'", got, want)
			}

		})
	}
}

func TestFailedStorage(t *testing.T) {
	for _, test := range []struct {
		desc      string
		wantError string
	}{
		{
			desc:      "Handle Storage failure",
			wantError: "failed to open file: open : no such file or directory",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			_, err := NewStorage(undefFp)
			if err == nil {
				t.Error("Unexpected success in storage creation", err)
			}
			fmt.Printf("Received Error = %s", err.Error())
			if err.Error() != test.wantError {
				t.Error("Unexpected error message received", err)
			}
		})
	}
}

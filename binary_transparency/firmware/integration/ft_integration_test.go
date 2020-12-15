// Copyright 2020 Google LLC. All Rights Reserved.
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

// Package integration is an integration test for the FT demo.
package integration_test

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/client/rpcflags"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"google.golang.org/grpc"

	i_emu "github.com/google/trillian-examples/binary_transparency/firmware/cmd/emulator/dummy/impl"
	i_flash "github.com/google/trillian-examples/binary_transparency/firmware/cmd/flash_tool/impl"
	i_personality "github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/impl"
	i_modify "github.com/google/trillian-examples/binary_transparency/firmware/cmd/hacker/modify_bundle/impl"
	i_publish "github.com/google/trillian-examples/binary_transparency/firmware/cmd/publisher/impl"
)

const (
	PublishTimestamp1       = "2020-11-24 10:00:00+00:00"
	PublishTimestamp2       = "2020-11-24 10:15:00+00:00"
	PublishMalwareTimestamp = "2020-11-24 10:30:00+00:00"

	GoodFirmware   = "../testdata/firmware/dummy_device/example.wasm"
	HackedFirmware = "../testdata/firmware/dummy_device/hacked.wasm"
)

var (
	trillianAddr = flag.String("trillian", "", "Host:port of Trillian Log RPC server")
)

func TestFTIntegration(t *testing.T) {
	if len(*trillianAddr) == 0 {
		t.Skip("--trillian flag unset, skipping test")
	}

	// Set up storage
	tmpDir := t.TempDir()
	updatePath := filepath.Join(tmpDir, "update.ota")
	devStoragePath := filepath.Join(tmpDir, "dummy_device")
	if err := os.MkdirAll(devStoragePath, 0755); err != nil {
		t.Fatalf("Failed to create device storage dir %q: %q", devStoragePath, err)
	}

	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		ctxD, c := context.WithDeadline(context.Background(), deadline)
		ctx = ctxD
		defer c()
	}
	tree := createTree(ctx, t)

	// TODO(al): make this dynamic
	pListen := "localhost:43563"
	pAddr := fmt.Sprintf("http://%s", pListen)

	go func() {
		if err := runPersonality(ctx, t, pListen, tree.TreeId); err != nil {
			t.Fatalf("Failed to run personality: %q", err)
		}
	}()

	// TODO(al): make this wait until the personality is listening
	<-time.After(5 * time.Second)

	for _, step := range []struct {
		desc    string
		step    func() error
		wantErr bool
	}{
		{
			desc: "Log initial firmware",
			step: func() error {
				return i_publish.Main(ctx, i_publish.PublishOpts{
					LogURL:     pAddr,
					DeviceID:   "dummy",
					BinaryPath: GoodFirmware,
					Timestamp:  PublishTimestamp1,
					Revision:   1,
					OutputPath: updatePath,
				})
			},
		}, {
			desc: "Force flashing device (init)",
			step: func() error {
				return i_flash.Main(i_flash.FlashOpts{
					LogURL:        pAddr,
					DeviceID:      "dummy",
					UpdateFile:    updatePath,
					DeviceStorage: devStoragePath,
					Force:         true,
				})
			},
		}, {
			desc: "Boot device with initial firmware",
			step: func() error {
				return i_emu.Main(i_emu.EmulatorOpts{
					DeviceStorage: devStoragePath,
				})
			},
		}, {
			desc: "Log updated firmware",
			step: func() error {
				return i_publish.Main(ctx, i_publish.PublishOpts{
					LogURL:     pAddr,
					DeviceID:   "dummy",
					BinaryPath: GoodFirmware,
					Timestamp:  PublishTimestamp2,
					Revision:   2,
					OutputPath: updatePath,
				})
			},
		}, {
			desc: "Flashing device (update)",
			step: func() error {
				return i_flash.Main(i_flash.FlashOpts{
					LogURL:        pAddr,
					DeviceID:      "dummy",
					UpdateFile:    updatePath,
					DeviceStorage: devStoragePath,
				})
			},
		}, {
			desc: "Booting updated device",
			step: func() error {
				return i_emu.Main(i_emu.EmulatorOpts{
					DeviceStorage: devStoragePath,
				})
			},
		}, {
			desc:    "Replace FW, boot device",
			wantErr: true,
			step: func() error {
				if err := copyFile(HackedFirmware, filepath.Join(devStoragePath, "firmware.bin")); err != nil {
					t.Fatalf("Failed to overwrite stored firmware: %q", err)
				}
				// Booting this should return an error:
				return i_emu.Main(i_emu.EmulatorOpts{
					DeviceStorage: devStoragePath,
				})
			},
		}, {
			desc:    "Replace FW, update hash (but not sign), and boot",
			wantErr: true,
			step: func() error {
				if err := copyFile(HackedFirmware, filepath.Join(devStoragePath, "firmware.bin")); err != nil {
					t.Fatalf("Failed to overwrite stored firmware: %q", err)
				}

				if err := i_modify.Main(i_modify.ModifyBundleOpts{
					BinaryPath: HackedFirmware,
					DeviceID:   "dummy",
					Input:      filepath.Join(devStoragePath, "bundle.json"),
					Output:     filepath.Join(devStoragePath, "bundle.json"),
				}); err != nil {
					t.Fatalf("Failed to modify bundle: %q", err)
				}

				// Booting this should return an error:
				return i_emu.Main(i_emu.EmulatorOpts{
					DeviceStorage: devStoragePath,
				})
			},
		}, {
			desc:    "Replace FW, update hash, sign manifest, and boot",
			wantErr: true,
			step: func() error {
				if err := copyFile(HackedFirmware, filepath.Join(devStoragePath, "firmware.bin")); err != nil {
					t.Fatalf("Failed to overwrite stored firmware: %q", err)
				}

				if err := i_modify.Main(i_modify.ModifyBundleOpts{
					BinaryPath: HackedFirmware,
					DeviceID:   "dummy",
					Input:      filepath.Join(devStoragePath, "bundle.json"),
					Output:     filepath.Join(devStoragePath, "bundle.json"),
					Sign:       true,
				}); err != nil {
					t.Fatalf("Failed to modify bundle: %q", err)
				}

				// Booting this should return an error:
				return i_emu.Main(i_emu.EmulatorOpts{
					DeviceStorage: devStoragePath,
				})
			},
		}, {
			desc: "Log malware, device boots, but monitor sees all!",
			step: func() error {
				// Log malware fw:
				if err := i_publish.Main(ctx, i_publish.PublishOpts{
					LogURL:     pAddr,
					DeviceID:   "dummy",
					BinaryPath: HackedFirmware,
					Timestamp:  PublishMalwareTimestamp,
					Revision:   1,
					OutputPath: updatePath,
				}); err != nil {
					t.Fatalf("Failed to log malware: %q", err)
				}

				// Now flash the bundle normally, it will install because it's been logged
				// and so is now discoverable.
				if err := i_flash.Main(i_flash.FlashOpts{
					LogURL:        pAddr,
					DeviceID:      "dummy",
					UpdateFile:    updatePath,
					DeviceStorage: devStoragePath,
				}); err != nil {
					t.Fatalf("Failed to flash malware update onto device: %q", err)
				}

				// Booting should also succeed:
				return i_emu.Main(i_emu.EmulatorOpts{
					DeviceStorage: devStoragePath,
				})

				// TODO(al): run monitor and verify it saw the malware
			},
		},
	} {
		t.Run(step.desc, func(t *testing.T) {
			err := step.step()
			if step.wantErr && err == nil {
				t.Fatal("Want error, got no error")
			} else if !step.wantErr && err != nil {
				t.Fatalf("Want no error, got %q", err)
			}
			if err != nil {
				t.Logf("Got expected error: %q", err)
			}
			// TODO(al): output matching
		})
	}
}

func createTree(ctx context.Context, t *testing.T) *trillian.Tree {
	t.Helper()
	ctr := &trillian.CreateTreeRequest{
		Tree: &trillian.Tree{
			TreeState:          trillian.TreeState_ACTIVE,
			TreeType:           trillian.TreeType_LOG,
			HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
			HashAlgorithm:      sigpb.DigitallySigned_SHA256,
			SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			DisplayName:        "FT integration test",
			Description:        "FT integration test log",
			MaxRootDuration:    ptypes.DurationProto(time.Hour),
		},
		KeySpec: &keyspb.Specification{
			Params: &keyspb.Specification_EcdsaParams{
				EcdsaParams: &keyspb.Specification_ECDSA{},
			},
		},
	}

	dialOpts, err := rpcflags.NewClientDialOptionsFromFlags()
	if err != nil {
		t.Fatalf("Failed to determine dial options: %v", err)
	}

	conn, err := grpc.Dial(*trillianAddr, dialOpts...)
	if err != nil {
		t.Fatalf("Failed to dial %v: %v", *trillianAddr, err)
	}
	defer conn.Close()

	adminClient := trillian.NewTrillianAdminClient(conn)
	mapClient := trillian.NewTrillianMapClient(conn)
	logClient := trillian.NewTrillianLogClient(conn)

	tree, err := client.CreateAndInitTree(ctx, ctr, adminClient, mapClient, logClient)
	if err != nil {
		t.Fatalf("Failed to create tree: %v", err)
	}
	t.Logf("Created tree ID %d", tree.TreeId)
	return tree
}

func runPersonality(ctx context.Context, t *testing.T, serverAddr string, treeID int64) error {
	t.Helper()
	r := t.TempDir()

	err := i_personality.Main(ctx, i_personality.PersonalityOpts{
		ListenAddr:     serverAddr,
		TreeID:         treeID,
		CASFile:        filepath.Join(r, "ft-cas.db"),
		TrillianAddr:   *trillianAddr,
		ConnectTimeout: 10 * time.Second,
		STHRefresh:     time.Second,
	})
	if err != http.ErrServerClosed {
		return err
	}
	return nil
}

func copyFile(from, to string) error {
	i, err := ioutil.ReadFile(from)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(to, i, 0644)
}

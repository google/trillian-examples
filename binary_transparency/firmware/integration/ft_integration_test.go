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

	"github.com/google/trillian-examples/binary_transparency/firmware/api"
	i_emu "github.com/google/trillian-examples/binary_transparency/firmware/cmd/emulator/dummy/impl"
	i_flash "github.com/google/trillian-examples/binary_transparency/firmware/cmd/flash_tool/impl"
	i_monitor "github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_monitor/impl"
	i_personality "github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_personality/impl"
	i_witness "github.com/google/trillian-examples/binary_transparency/firmware/cmd/ft_witness/impl"
	i_modify "github.com/google/trillian-examples/binary_transparency/firmware/cmd/hacker/modify_bundle/impl"
	i_publish "github.com/google/trillian-examples/binary_transparency/firmware/cmd/publisher/impl"
)

const (
	PublishTimestamp1       = "2020-11-24 10:00:00+00:00"
	PublishTimestamp2       = "2020-11-24 10:15:00+00:00"
	PublishTimestamp3       = "2021-02-16 10:00:00+00:00"
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

	tmpDir := t.TempDir()
	updatePath := filepath.Join(tmpDir, "update.ota")
	devStoragePath := filepath.Join(tmpDir, "dummy_device")
	setupDeviceStorage(t, devStoragePath)

	ctx, cancel := testContext(t)
	defer cancel()

	// TODO(al): make this dynamic
	pListen := "localhost:43563"
	pAddr := fmt.Sprintf("http://%s", pListen)

	pErrChan := make(chan error)

	go func() {
		if err := runPersonality(ctx, t, pListen); err != nil {
			pErrChan <- err
		}
		close(pErrChan)
	}()

	// TODO(al): make this wait until the personality is listening
	<-time.After(5 * time.Second)

	for _, step := range []struct {
		desc       string
		step       func() error
		wantErrMsg string
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
				return i_flash.Main(ctx, i_flash.FlashOpts{
					LogURL:        pAddr,
					WitnessURL:    "",
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
				return i_flash.Main(ctx, i_flash.FlashOpts{
					LogURL:        pAddr,
					WitnessURL:    "",
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
			desc:       "Replace FW, boot device",
			wantErrMsg: "firmware measurement does not match",
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
			desc:       "Replace FW, update hash (but not sign), and boot",
			wantErrMsg: "failed to verify signature",
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
			desc:       "Replace FW, update hash, sign manifest, and boot",
			wantErrMsg: "invalid inclusion proof in bundle",
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

				// Start up the monitor:
				mErrChan := make(chan error, 1)
				matchedChan := make(chan bool, 1)
				mCtx, mCancel := context.WithCancel(context.Background())
				defer mCancel()
				go func() {
					if err := runMonitor(mCtx, t, pAddr, "H4x0r3d", func(idx uint64, fw api.FirmwareMetadata) {
						t.Logf("Found malware firmware @%d", idx)
						matchedChan <- true
					}); err != nil && err != context.Canceled {
						mErrChan <- err
					}
					close(mErrChan)
				}()

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

				<-time.After(5 * time.Second)
				// Now flash the bundle normally, it will install because it's been logged
				// and so is now discoverable.
				if err := i_flash.Main(ctx, i_flash.FlashOpts{
					LogURL:        pAddr,
					WitnessURL:    "",
					DeviceID:      "dummy",
					UpdateFile:    updatePath,
					DeviceStorage: devStoragePath,
				}); err != nil {
					t.Fatalf("Failed to flash malware update onto device: %q", err)
				}

				// Booting should also succeed:
				if err := i_emu.Main(i_emu.EmulatorOpts{
					DeviceStorage: devStoragePath,
				}); err != nil {
					t.Fatalf("Failed to boot device with logged malware: %q", err)
				}

				// Wait and see if the monitor spots the malware
				select {
				case <-time.After(30 * time.Second):
					t.Fatal("Monitor didn't spot logged malware")
				case err := <-mErrChan:
					t.Fatalf("Monitor errored: %q", err)
				case <-matchedChan:
					// We found it
				}

				return nil
			},
		}, {
			desc: "Firmware update with witness verification",
			step: func() error {
				// Start up the witness:
				wHost := "localhost:43565"
				wAddr := fmt.Sprintf("http://%s", wHost)
				wCtx, wCancel := context.WithCancel(context.Background())
				wErrChan := make(chan error)
				defer wCancel()
				go func() {
					if err := runWitness(wCtx, t, pAddr, wHost); err != nil {
						pErrChan <- err
					}
					close(wErrChan)
				}()

				// Wait for few seconds before starting the test
				<-time.After(2 * time.Second)
				if err := i_publish.Main(ctx, i_publish.PublishOpts{
					LogURL:     pAddr,
					DeviceID:   "dummy",
					BinaryPath: GoodFirmware,
					Timestamp:  PublishTimestamp3,
					Revision:   3,
					OutputPath: updatePath,
				}); err != nil {
					t.Fatalf("Failed to publish new bundle: %q", err)
				}

				// Wait witness to view the device checkpoint
				<-time.After(5 * time.Second)

				if err := i_flash.Main(ctx, i_flash.FlashOpts{
					LogURL:        pAddr,
					WitnessURL:    wAddr,
					DeviceID:      "dummy",
					UpdateFile:    updatePath,
					DeviceStorage: devStoragePath,
				}); err != nil {
					t.Fatalf("witness verification failed: %q", err)
				}
				return nil
			},
		},
	} {
		t.Run(step.desc, func(t *testing.T) {
			wantErr := len(step.wantErrMsg) > 0
			err := step.step()
			if wantErr && err == nil {
				t.Fatal("Want error, got no error")
			} else if !wantErr && err != nil {
				t.Fatalf("Want no error, got %q", err)
			}
			if err != nil {
				t.Logf("Got expected error: %q", err)
			}
			// TODO(al): output matching
		})
	}
}

func testContext(t *testing.T) (context.Context, func()) {
	ctx := context.Background()
	c := func() {}
	if deadline, ok := t.Deadline(); ok {
		ctx, c = context.WithDeadline(context.Background(), deadline)
	}
	return ctx, c
}

func setupDeviceStorage(t *testing.T, devStoragePath string) {
	t.Helper()
	if err := os.MkdirAll(devStoragePath, 0755); err != nil {
		t.Fatalf("Failed to create device storage dir %q: %q", devStoragePath, err)
	}
}

func runPersonality(ctx context.Context, t *testing.T, serverAddr string) error {

	t.Helper()
	r := t.TempDir()

	err := i_personality.Main(ctx, i_personality.PersonalityOpts{
		ListenAddr:     serverAddr,
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

func runWitness(ctx context.Context, t *testing.T, persAddr, serverAddr string) error {
	t.Helper()
	r := t.TempDir()

	err := i_witness.Main(ctx, i_witness.WitnessOpts{
		ListenAddr:   serverAddr,
		WSFile:       filepath.Join(r, "ft-witness.db"),
		FtLogURL:     persAddr,
		PollInterval: 5 * time.Second,
	})
	if err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runMonitor(ctx context.Context, t *testing.T, serverAddr string, pattern string, matched i_monitor.MatchFunc) error {
	t.Helper()

	r := t.TempDir()

	err := i_monitor.Main(ctx, i_monitor.MonitorOpts{
		LogURL:       serverAddr,
		PollInterval: 1 * time.Second,
		Keyword:      "H4x0r3d",
		Matched:      matched,
		StateFile:    filepath.Join(r, "ft-monitor.state"),
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

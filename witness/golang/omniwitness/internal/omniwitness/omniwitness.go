// Copyright 2022 Google LLC. All Rights Reserved.
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

// omniwitness provides a single Main file that basically runs the omniwitness.
// Some components are left pluggable so this can be deployed on different
// runtimes.
package omniwitness

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/google/trillian-examples/serverless/config"
	wimpl "github.com/google/trillian-examples/witness/golang/cmd/witness/impl"
	"github.com/google/trillian-examples/witness/golang/internal/persistence/inmemory"
	"github.com/google/trillian-examples/witness/golang/internal/witness"
	"github.com/google/trillian-examples/witness/golang/omniwitness"
	"github.com/gorilla/mux"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"

	"github.com/google/trillian-examples/internal/feeder"
	"github.com/google/trillian-examples/internal/feeder/pixelbt"
	"github.com/google/trillian-examples/internal/feeder/rekor"
	"github.com/google/trillian-examples/internal/feeder/serverless"
	"github.com/google/trillian-examples/internal/feeder/sumdb"
)

const (
	// Interval between attempts to feed checkpoints
	// TODO(mhutchinson): Make this configurable
	feedInterval = 5 * time.Minute
)

// singleLogFeederConfig encapsulates the feeder config for a feeder that can only
// feed a single log.
type singleLogFeederConfig struct {
	// Log defines the source log to feed from.
	Log config.Log `yaml:"Log"`
}

// multiLogFeederConfig encapsulates the feeder config for a feeder that can support
// multiple logs.
// TODO(mhutchinson): why do we do this to ourselves with similar but different configs?!
// See if we can standardize on them all being plural.
type multiLogFeederConfig struct {
	// Log defines the source log to feed from.
	Logs []config.Log `yaml:"Logs"`
}

// Main runs the omniwitness, with the witness listening using the listener, and all
// outbound HTTP calls using the client provided.
func Main(ctx context.Context, signer note.Signer, httpListener net.Listener, httpClient *http.Client) error {
	// This error group will be used to run all top level processes
	g := errgroup.Group{}

	type logFeeder func(context.Context, config.Log, feeder.Witness, *http.Client, time.Duration) error
	feeders := make(map[config.Log]logFeeder)

	// Feeder: SumDB
	sumdbFeederConfig := multiLogFeederConfig{}
	if err := yaml.Unmarshal(omniwitness.ConfigFeederSumDB, &sumdbFeederConfig); err != nil {
		return fmt.Errorf("failed to unmarshal sumdb config: %v", err)
	}
	for _, l := range sumdbFeederConfig.Logs {
		feeders[l] = sumdb.FeedLog
	}

	// Feeder: PixelBT
	pixelFeederConfig := singleLogFeederConfig{}
	if err := yaml.Unmarshal(omniwitness.ConfigFeederPixel, &pixelFeederConfig); err != nil {
		return fmt.Errorf("failed to unmarshal pixel config: %v", err)
	}
	feeders[pixelFeederConfig.Log] = pixelbt.FeedLog

	// Feeder: Rekor
	rekorFeederConfig := multiLogFeederConfig{}
	if err := yaml.Unmarshal(omniwitness.ConfigFeederRekor, &rekorFeederConfig); err != nil {
		return fmt.Errorf("failed to unmarshal rekor config: %v", err)
	}
	for _, l := range rekorFeederConfig.Logs {
		feeders[l] = rekor.FeedLog
	}

	// Feeder: Serverless
	serverlessFeederConfig := multiLogFeederConfig{}
	if err := yaml.Unmarshal(omniwitness.ConfigFeederServerless, &serverlessFeederConfig); err != nil {
		return fmt.Errorf("failed to unmarshal serverless config: %v", err)
	}
	for _, l := range serverlessFeederConfig.Logs {
		feeders[l] = serverless.FeedLog
	}

	// Witness
	witCfg := wimpl.LogConfig{}
	if err := yaml.Unmarshal(omniwitness.ConfigWitness, &witCfg); err != nil {
		return fmt.Errorf("failed to unmarshal witness config: %v", err)
	}
	knownLogs, err := witCfg.AsLogMap()
	if err != nil {
		return fmt.Errorf("failed to convert witness config to map: %v", err)
	}
	witness, err := witness.New(witness.Opts{
		Persistence: inmemory.NewPersistence(),
		Signer:      signer,
		KnownLogs:   knownLogs,
	})
	if err != nil {
		return fmt.Errorf("failed to create witness: %v", err)
	}

	bw := witnessAdapter{
		w: witness,
	}
	for c, f := range feeders {
		c, f := c, f
		// Continually feed this log in its own goroutine, hooked up to the global waitgroup.
		g.Go(func() error {
			return f(ctx, c, bw, httpClient, feedInterval)
		})
	}

	// TODO(mhutchinson): Start the distributors too if auth details are present.

	s := server{
		w: witness,
	}
	g.Go(func() error {
		return s.run(ctx, httpListener)
	})

	return g.Wait()
}

// server binds the witness to HTTP interface.
// TODO(mhutchinson): consider de-duping with the version in cmd/witness/internal/http
type server struct {
	w *witness.Witness
}

// getCheckpoint returns a checkpoint stored for a given log.
func (s *server) getCheckpoint(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	logID := v["logid"]
	// Get the signed checkpoint from the witness.
	chkpt, err := s.w.GetCheckpoint(logID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get checkpoint: %v", err), httpForCode(status.Code(err)))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Write(chkpt)
}

// getLogs returns a list of all logs the witness is aware of.
func (s *server) getLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.w.GetLogs()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get log list: %v", err), http.StatusInternalServerError)
		return
	}
	logList, err := json.Marshal(logs)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to convert log list to JSON: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/json")
	w.Write(logList)
}

func (s *server) run(ctx context.Context, l net.Listener) error {
	r := mux.NewRouter()
	r.HandleFunc("/", s.getLogs)
	r.HandleFunc("/{logid}", s.getCheckpoint)
	srv := http.Server{
		Handler: r,
	}
	e := make(chan error, 1)
	go func() {
		e <- srv.Serve(l)
		close(e)
	}()
	<-ctx.Done()
	srv.Shutdown(ctx)
	return <-e
}

func httpForCode(c codes.Code) int {
	switch c {
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.NotFound:
		return http.StatusNotFound
	case codes.FailedPrecondition, codes.InvalidArgument:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// witnessAdapter binds the internal witness implementation to the feeder interface.
// TODO(mhutchinson): Can we fix the difference between the API on the client and impl
// so they both have the same contract?
type witnessAdapter struct {
	w *witness.Witness
}

func (w witnessAdapter) GetLatestCheckpoint(ctx context.Context, logID string) ([]byte, error) {
	cp, err := w.w.GetCheckpoint(logID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, os.ErrNotExist
		}
	}
	return cp, err
}

func (w witnessAdapter) Update(ctx context.Context, logID string, newCP []byte, proof [][]byte) ([]byte, error) {
	return w.w.Update(ctx, logID, newCP, proof)
}

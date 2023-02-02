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

// Package omniwitness provides a single Main file that runs the omniwitness.
// Some components are left pluggable so this can be deployed on different
// runtimes.
package omniwitness

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/serverless/config"
	wimpl "github.com/google/trillian-examples/witness/golang/cmd/witness/impl"
	ihttp "github.com/google/trillian-examples/witness/golang/internal/http"
	"github.com/google/trillian-examples/witness/golang/internal/persistence"
	"github.com/google/trillian-examples/witness/golang/internal/witness"
	"github.com/gorilla/mux"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"

	dist_gh "github.com/google/trillian-examples/internal/distribute/github"
	"github.com/google/trillian-examples/internal/feeder"
	"github.com/google/trillian-examples/internal/feeder/pixelbt"
	"github.com/google/trillian-examples/internal/feeder/rekor"
	"github.com/google/trillian-examples/internal/feeder/serverless"
	"github.com/google/trillian-examples/internal/feeder/sumdb"
	"github.com/google/trillian-examples/internal/github"
	i_note "github.com/google/trillian-examples/internal/note"
)

type LogStatePersistence = persistence.LogStatePersistence
type LogStateReadOps = persistence.LogStateReadOps
type LogStateWriteOps = persistence.LogStateWriteOps

const (
	// Interval between attempts to feed checkpoints
	// TODO(mhutchinson): Make this configurable
	feedInterval       = 5 * time.Minute
	distributeInterval = 5 * time.Minute
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

// OperatorConfig allows the bare minimum operator-specific configuration.
// This should only contain configuration details that are custom per-operator.
type OperatorConfig struct {
	WitnessSigner   note.Signer
	WitnessVerifier note.Verifier

	GithubUser  string
	GithubEmail string
	GithubToken string
}

// Main runs the omniwitness, with the witness listening using the listener, and all
// outbound HTTP calls using the client provided.
func Main(ctx context.Context, operatorConfig OperatorConfig, p LogStatePersistence, httpListener net.Listener, httpClient *http.Client) error {
	// This error group will be used to run all top level processes.
	// If any process dies, then all of them will be stopped via context cancellation.
	g, ctx := errgroup.WithContext(ctx)

	type logFeeder func(context.Context, config.Log, feeder.Witness, *http.Client, time.Duration) error
	feeders := make(map[config.Log]logFeeder)

	// Feeder: SumDB
	sumdbFeederConfig := multiLogFeederConfig{}
	if err := yaml.Unmarshal(ConfigFeederSumDB, &sumdbFeederConfig); err != nil {
		return fmt.Errorf("failed to unmarshal sumdb config: %v", err)
	}
	for _, l := range sumdbFeederConfig.Logs {
		feeders[l] = sumdb.FeedLog
	}

	// Feeder: PixelBT
	pixelFeederConfig := singleLogFeederConfig{}
	if err := yaml.Unmarshal(ConfigFeederPixel, &pixelFeederConfig); err != nil {
		return fmt.Errorf("failed to unmarshal pixel config: %v", err)
	}
	feeders[pixelFeederConfig.Log] = pixelbt.FeedLog

	// Feeder: Rekor
	rekorFeederConfig := multiLogFeederConfig{}
	if err := yaml.Unmarshal(ConfigFeederRekor, &rekorFeederConfig); err != nil {
		return fmt.Errorf("failed to unmarshal rekor config: %v", err)
	}
	for _, l := range rekorFeederConfig.Logs {
		feeders[l] = rekor.FeedLog
	}

	// Feeder: Serverless
	serverlessFeederConfig := multiLogFeederConfig{}
	if err := yaml.Unmarshal(ConfigFeederServerless, &serverlessFeederConfig); err != nil {
		return fmt.Errorf("failed to unmarshal serverless config: %v", err)
	}
	for _, l := range serverlessFeederConfig.Logs {
		feeders[l] = serverless.FeedLog
	}

	// Witness
	witCfg := wimpl.LogConfig{}
	if err := yaml.Unmarshal(ConfigWitness, &witCfg); err != nil {
		return fmt.Errorf("failed to unmarshal witness config: %v", err)
	}
	knownLogs, err := witCfg.AsLogMap()
	if err != nil {
		return fmt.Errorf("failed to convert witness config to map: %v", err)
	}
	witness, err := witness.New(witness.Opts{
		Persistence: p,
		Signer:      operatorConfig.WitnessSigner,
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
			glog.Infof("Feeder %q goroutine started", c.Origin)
			defer glog.Infof("Feeder %q goroutine done", c.Origin)
			return f(ctx, c, bw, httpClient, feedInterval)
		})
	}

	if operatorConfig.GithubUser != "" {
		var distLogs = []dist_gh.Log{}
		for config := range feeders {
			// TODO(mhutchinson): This verifier could be created inside the distributor.
			logSigV, err := i_note.NewVerifier(config.PublicKeyType, config.PublicKey)
			if err != nil {
				return err
			}
			distLogs = append(distLogs, dist_gh.Log{
				Config: config,
				SigV:   logSigV,
			})
		}

		runDistributors(ctx, httpClient, g, distLogs, bw, operatorConfig)
	} else {
		glog.Info("No GitHub user specified; skipping deployment of distributors")
	}

	r := mux.NewRouter()
	s := ihttp.NewServer(witness)
	s.RegisterHandlers(r)
	srv := http.Server{
		Handler: r,
	}
	g.Go(func() error {
		glog.Info("HTTP server goroutine started")
		defer glog.Info("HTTP server goroutine done")
		return srv.Serve(httpListener)
	})
	g.Go(func() error {
		// This goroutine brings down the HTTP server when ctx is done.
		glog.Info("HTTP server-shutdown goroutine started")
		defer glog.Info("HTTP server-shutdown goroutine done")
		<-ctx.Done()
		return srv.Shutdown(ctx)
	})

	return g.Wait()
}

func runDistributors(ctx context.Context, c *http.Client, g *errgroup.Group, logs []dist_gh.Log, witness dist_gh.Witness, operatorConfig OperatorConfig) {
	distribute := func(opts *dist_gh.DistributeOptions) error {
		if err := dist_gh.DistributeOnce(ctx, opts); err != nil {
			glog.Errorf("DistributeOnce: %v", err)
		}
		for {
			select {
			case <-time.After(distributeInterval):
			case <-ctx.Done():
				return ctx.Err()
			}
			if err := dist_gh.DistributeOnce(ctx, opts); err != nil {
				glog.Errorf("DistributeOnce: %v", err)
			}
		}
	}

	createOpts := func(upstreamOwner, repoName, mainBranch, distributorPath string) (*dist_gh.DistributeOptions, error) {
		dr := github.RepoID{
			Owner:    upstreamOwner,
			RepoName: repoName,
		}
		fork := github.RepoID{
			Owner:    operatorConfig.GithubUser,
			RepoName: repoName,
		}
		repo, err := github.NewRepository(ctx, c, dr, mainBranch, fork, operatorConfig.GithubUser, operatorConfig.GithubEmail, operatorConfig.GithubToken)
		if err != nil {
			return nil, fmt.Errorf("NewRepository: %v", err)
		}
		return &dist_gh.DistributeOptions{
			Repo:            repo,
			DistributorPath: distributorPath,
			Logs:            logs,
			WitSigV:         operatorConfig.WitnessVerifier,
			Witness:         witness,
		}, nil
	}

	g.Go(func() error {
		opts, err := createOpts("mhutchinson", "mhutchinson-distributor", "main", "distributor")
		if err != nil {
			return fmt.Errorf("createOpts(mhutchinson): %v", err)
		}
		glog.Infof("Distributor %q goroutine started", opts.Repo)
		defer glog.Infof("Distributor %q goroutine done", opts.Repo)
		return distribute(opts)
	})

	g.Go(func() error {
		opts, err := createOpts("WolseyBankWitness", "rediffusion", "main", ".")
		if err != nil {
			return fmt.Errorf("createOpts(WolseyBankWitness): %v", err)
		}
		glog.Infof("Distributor %q goroutine started", opts.Repo)
		defer glog.Infof("Distributor %q goroutine done", opts.Repo)
		return distribute(opts)
	})
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

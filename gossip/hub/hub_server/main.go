// Copyright 2018 Google Inc. All Rights Reserved.
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

// The hub_server binary runs the Gossip Hub personality.
package main

import (
	"context"
	"flag"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/certificate-transparency-go/x509"
	"github.com/google/trillian"
	"github.com/google/trillian-examples/gossip/hub"
	"github.com/google/trillian-examples/gossip/hub/configpb"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/google/trillian/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/tomasen/realip"
	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	// Register PEMKeyFile, PrivateKey and PKCS11Config ProtoHandlers
	_ "github.com/google/trillian/crypto/keys/der/proto"
	_ "github.com/google/trillian/crypto/keys/pem/proto"
	_ "github.com/google/trillian/crypto/keys/pkcs11/proto"
)

// Global flags that affect all Hub instances.
var (
	httpEndpoint    = flag.String("http_endpoint", "localhost:16962", "Endpoint for HTTP (host:port)")
	metricsEndpoint = flag.String("metrics_endpoint", "localhost:16963", "Endpoint for serving metrics; if left empty, metrics will be visible on --http_endpoint")
	rpcBackend      = flag.String("log_rpc_server", "localhost:8090", "Backend specification; comma-separated list. If unset backends are specified in config (as a LogMultiConfig proto)")
	rpcDeadline     = flag.Duration("rpc_deadline", time.Second*10, "Deadline for backend RPC requests")
	getSLRInterval  = flag.Duration("get_slr_interval", time.Second*180, "Interval between internal get-slr operations (0 to disable)")
	hubConfig       = flag.String("hub_config", "", "File holding Hub config in text proto format")
	maxGetEntries   = flag.Int64("max_get_entries", 0, "Max number of entries we allow in a get-entries request (0=>use default 1000)")
	quotaRemote     = flag.Bool("quota_remote", true, "Enable requesting of quota for IP address sending incoming requests")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	var cfg *configpb.HubMultiConfig
	var err error
	// Get Hub config from file before we start. This is a different proto
	// type if we're using a multi backend configuration (no rpcBackend set
	// in flags). The single-backend config is converted to a multi config so
	// they can be treated the same.
	if len(*rpcBackend) > 0 {
		cfg, err = hub.ConfigFromSingleFile(*hubConfig, *rpcBackend)
	} else {
		cfg, err = hub.ConfigFromMultiFile(*hubConfig)
	}

	if err != nil {
		glog.Exitf("Failed to read config: %v", err)
	}

	if err := hub.ValidateHubMultiConfig(cfg); err != nil {
		glog.Exitf("Invalid config: %v", err)
	}

	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Hub Server Starting ****")

	metricsAt := *metricsEndpoint
	if metricsAt == "" {
		metricsAt = *httpEndpoint
	}

	dialOpts := []grpc.DialOption{grpc.WithInsecure()}
	if strings.Contains(*rpcBackend, ",") {
		// This should probably not be used in production. Either use etcd or a gRPC
		// load balancer.
		glog.Warning("Multiple RPC backends from flags not recommended for production. Should probably be using etcd or a gRPC load balancer / proxy.")
		res, cleanup := manual.GenerateAndRegisterManualResolver()
		defer cleanup()
		backends := strings.Split(*rpcBackend, ",")
		addrs := make([]resolver.Address, 0, len(backends))
		for _, backend := range backends {
			addrs = append(addrs, resolver.Address{Addr: backend, Type: resolver.Backend})
		}
		res.InitialState(resolver.State{Addresses: addrs})
		resolver.SetDefaultScheme(res.Scheme())
	} else {
		glog.Infof("Using regular DNS resolver")
	}

	// Dial all our Trillian Log backends.
	clientMap := make(map[string]trillian.TrillianLogClient)
	for beName, beSpec := range cfg.HubBackends {
		glog.Infof("Dialling backend %q at %v", beName, beSpec)
		if len(cfg.HubBackends) == 1 {
			// If there's only one of them we use the blocking option as we can't
			// serve anything until connected.
			dialOpts = append(dialOpts, grpc.WithBlock())
		}
		conn, err := grpc.Dial(beSpec, dialOpts...)
		if err != nil {
			glog.Exitf("Could not dial RPC server: %v: %v", beSpec, err)
		}
		defer conn.Close()
		clientMap[beName] = trillian.NewTrillianLogClient(conn)
	}

	// Allow cross-origin requests to all handlers registered on corsMux.
	// This is safe for gossip hub handlers because the hub is public and
	// unauthenticated so cross-site scripting attacks are not a concern.
	corsMux := http.NewServeMux()
	corsHandler := cors.AllowAll().Handler(corsMux)
	http.Handle("/", corsHandler)

	// Register handlers for all the configured hubs using the correct RPC client.
	for _, c := range cfg.HubConfig {
		if err := setupAndRegister(ctx, corsMux, clientMap[c.BackendName], *rpcDeadline, *maxGetEntries, c); err != nil {
			glog.Exitf("Failed to set up Hub instance for %+v: %v", cfg, err)
		}
	}

	// Return a 200 on the root, for anything that checks health there (e.g. GCE).
	corsMux.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/" {
			resp.WriteHeader(http.StatusOK)
		} else {
			resp.WriteHeader(http.StatusNotFound)
		}
	})

	if metricsAt != *httpEndpoint {
		// Run a separate handler for metrics.
		go func() {
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			metricsServer := http.Server{Addr: metricsAt, Handler: mux}
			err := metricsServer.ListenAndServe()
			glog.Warningf("Metrics server exited: %v", err)
		}()
	} else {
		// Handle metrics on the DefaultServeMux.
		http.Handle("/metrics", promhttp.Handler())
	}

	if *getSLRInterval > 0 {
		// Regularly update the internal SLR for each Trillian log so our metrics stay
		// up-to-date with any tree head changes that are not triggered by us.
		for _, c := range cfg.HubConfig {
			ticker := time.NewTicker(*getSLRInterval)
			go func(c *configpb.HubConfig) {
				trillianKey, err := x509.ParsePKIXPublicKey(c.TrillianKey.GetDer())
				if err != nil {
					glog.Warningf("No Trillian public key for log %v (%d)", c.Prefix, c.LogId)
				}
				glog.Infof("start internal get-slr operations on log %v (%d)", c.Prefix, c.LogId)
				for t := range ticker.C {
					glog.V(1).Infof("tick at %v: force internal get-slr for log %v (%d)", t, c.Prefix, c.LogId)
					if _, err := hub.GetLogRoot(ctx, clientMap[c.BackendName], trillianKey, c.LogId, c.Prefix); err != nil {
						glog.Warningf("failed to retrieve log root for log %v (%d): %v", c.Prefix, c.LogId, err)
					}
				}
			}(c)
		}
	}

	// Bring up the HTTP server and serve until we get a signal not to.
	server := http.Server{Addr: *httpEndpoint, Handler: nil}
	var shutdownWG sync.WaitGroup
	go util.AwaitSignal(ctx, func() {
		shutdownWG.Add(1)
		defer shutdownWG.Done()
		// Allow 60s for any pending requests to finish then terminate any stragglers
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
		defer cancel()
		glog.Info("Shutting down HTTP server...")
		server.Shutdown(ctx)
		glog.Info("HTTP server shutdown")
	})

	err = server.ListenAndServe()
	if err != http.ErrServerClosed {
		glog.Warningf("Server exited: %v", err)
	}
	// Wait will only block if the function passed to awaitSignal was called,
	// in which case it'll block until the HTTP server has gracefully shutdown
	shutdownWG.Wait()
	glog.Flush()
}

func setupAndRegister(ctx context.Context, mux *http.ServeMux, client trillian.TrillianLogClient, deadline time.Duration, maxGetEntries int64, cfg *configpb.HubConfig) error {
	opts := hub.InstanceOptions{
		Deadline:      deadline,
		MaxGetEntries: maxGetEntries,
		MetricFactory: prometheus.MetricFactory{},
	}
	if *quotaRemote {
		glog.Info("Enabling quota for requesting IP")
		opts.RemoteQuotaUser = realip.FromRequest
	}
	handlers, err := hub.SetUpInstance(ctx, client, cfg, opts)
	if err != nil {
		return err
	}
	for path, handler := range *handlers {
		mux.Handle(path, handler)
	}
	return nil
}

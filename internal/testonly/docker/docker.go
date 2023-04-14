// Copyright 2023 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package docker provides functions for integration tests that rely on Docker.
// Configuring tests via flags doesn't scale well (because passing that flag to
// ./... will fail for all of the other tests), and so we use OS environment
// variables as a way to allow for environment configuration in tests.
package docker

import (
	"fmt"
	"net"
	"os"

	"github.com/ory/dockertest/v3"
	dck "github.com/ory/dockertest/v3/docker"
)

const goTestDockerIntegration = "GO_TEST_DOCKER_INTEGRATION"

// ConfigureHost configures the host for a standard docker environment.
func ConfigureHost(config *dck.HostConfig) {
	if env, ok := os.LookupEnv(goTestDockerIntegration); ok {
		switch env {
		case "cloudbuild":
			config.NetworkMode = "cloudbuild"
		default:
			panic(fmt.Sprintf("Unsupported option for GO_TEST_DOCKER_INTEGRATION: %q", env))
		}
	}
}

// GetAddress returns the server address for the resource.
func GetAddress(r *dockertest.Resource) string {
	if env, ok := os.LookupEnv(goTestDockerIntegration); ok {
		switch env {
		case "cloudbuild":
			n := r.Container.NetworkSettings.Networks["cloudbuild"]
			return net.JoinHostPort(n.IPAddress, "3306")
		default:
			panic(fmt.Sprintf("Unsupported option for GO_TEST_DOCKER_INTEGRATION: %q", env))
		}
	}
	return net.JoinHostPort("localhost", r.GetPort("3306/tcp"))
}

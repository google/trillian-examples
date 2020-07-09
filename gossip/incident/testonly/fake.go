// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package testonly contains fakes for use in tests that interact with incident reporting.
package testonly

import (
	"context"
	"fmt"
)

// Report contains all of the information submitted when creating an incident report.
type Report struct {
	BaseURL string
	Summary string
	FullURL string
	Details string
}

// FakeReporter sends incident reports to its Reports channel.
type FakeReporter struct {
	Updates    chan Report
	Violations chan Report
}

// LogUpdate sends an incident report to the FakeReporter's Updates channel.
func (f *FakeReporter) LogUpdate(ctx context.Context, baseURL, summary, fullURL, details string) {
	f.Updates <- Report{
		BaseURL: baseURL,
		Summary: summary,
		FullURL: fullURL,
		Details: details,
	}
}

// LogUpdatef sends an incident report to the FakeReporter's Updates channel, formatting parameters along the way.
func (f *FakeReporter) LogUpdatef(ctx context.Context, baseURL, summary, fullURL, detailsFmt string, args ...interface{}) {
	f.LogUpdate(ctx, baseURL, summary, fullURL, fmt.Sprintf(detailsFmt, args...))
}

// LogViolation sends an incident report to the FakeReporter's Violations channel.
func (f *FakeReporter) LogViolation(ctx context.Context, baseURL, summary, fullURL, details string) {
	f.Violations <- Report{
		BaseURL: baseURL,
		Summary: summary,
		FullURL: fullURL,
		Details: details,
	}
}

// LogViolationf sends an incident report to the FakeReporter's Violations channel, formatting parameters along the way.
func (f *FakeReporter) LogViolationf(ctx context.Context, baseURL, summary, fullURL, detailsFmt string, args ...interface{}) {
	f.LogViolation(ctx, baseURL, summary, fullURL, fmt.Sprintf(detailsFmt, args...))
}

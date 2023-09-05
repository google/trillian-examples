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

package ftmap

import (
	"fmt"
	"testing"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/register"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/testing/passert"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/testing/ptest"
	"github.com/google/trillian-examples/binary_transparency/firmware/api"
)

func init() {
	register.Function1x1(testAggregationToStringFn)
}

func TestMain(m *testing.M) {
	ptest.Main(m)
}

func TestAggregate(t *testing.T) {
	fwEntries := []*firmwareLogEntry{
		{Index: 0, Firmware: createFW("dummy", 400)},
		{Index: 1, Firmware: createFW("dummy", 401)},
	}
	logHead := int64(len(fwEntries) - 1)
	createAnnotationMalware := func(fwIndex int, good bool) *annotationMalwareLogEntry {
		logHead++
		return &annotationMalwareLogEntry{
			Index: logHead,
			Annotation: api.MalwareStatement{
				FirmwareID: api.FirmwareID{
					LogIndex:            uint64(fwIndex),
					FirmwareImageSHA512: fwEntries[fwIndex].Firmware.FirmwareImageSHA512,
				},
				Good: good,
			},
		}
	}
	tests := []struct {
		name               string
		treeID             int64
		annotationMalwares []*annotationMalwareLogEntry

		wantGood []string
	}{
		{
			name:   "No annotations",
			treeID: 12345,

			wantGood: []string{"0: true", "1: true"},
		},
		{
			name:   "One bad annotation",
			treeID: 12345,

			annotationMalwares: []*annotationMalwareLogEntry{
				createAnnotationMalware(1, false),
			},

			wantGood: []string{"0: true", "1: false"},
		},
		{
			name:   "Many annotations all good",
			treeID: 12345,

			annotationMalwares: []*annotationMalwareLogEntry{
				createAnnotationMalware(0, true),
				createAnnotationMalware(0, true),
			},

			wantGood: []string{"0: true", "1: true"},
		},
		{
			name:   "Many annotations one bad",
			treeID: 12345,

			annotationMalwares: []*annotationMalwareLogEntry{
				createAnnotationMalware(0, true),
				createAnnotationMalware(0, false),
				createAnnotationMalware(0, true),
			},

			wantGood: []string{"0: false", "1: true"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			p, s := beam.NewPipelineWithRoot()

			fws := beam.CreateList(s, fwEntries)
			annotationMalwares := beam.CreateList(s, test.annotationMalwares)

			entries, aggs := Aggregate(s, test.treeID, fws, annotationMalwares)

			passert.Count(s, entries, "entries", len(fwEntries))
			passert.Count(s, aggs, "aggs", len(fwEntries))

			passert.Equals(s, beam.ParDo(s, testAggregationToStringFn, aggs), beam.CreateList(s, test.wantGood))

			err := ptest.Run(p)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func testAggregationToStringFn(a *api.AggregatedFirmware) string {
	return fmt.Sprintf("%d: %t", a.Index, a.Good)
}

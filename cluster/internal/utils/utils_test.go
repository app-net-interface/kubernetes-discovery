// Copyright (c) 2023 Cisco Systems, Inc. and its affiliates
// All rights reserved.
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
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"reflect"
	"testing"

	cluster "github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/panjf2000/ants"
	"github.com/stretchr/testify/assert"
)

func caseFailed(i int) string {
	return fmt.Sprintf("case %d failed", i)
}

func TestParseClusterRetrieveOptions(t *testing.T) {
	type testCase struct {
		in     []cluster.RetrieveOption
		exp    *cluster.RetrieveOptions
		expErr error
	}

	testChan := make(chan cluster.DiscoveredCluster, 1)
	testWorkerPool, err := ants.NewPool(1)
	if err != nil {
		t.Fatal(err)
	}
	cases := []testCase{
		{
			in: []cluster.RetrieveOption{
				cluster.WithRegions("region-1", "region-1", "-", "region-2"),
				cluster.WithRegions("region-1", "region-2", "region-3"),
			},
			exp: &cluster.RetrieveOptions{
				Regions: map[string]bool{
					"region-1": true,
					"region-2": true,
					"region-3": true,
				},
				MaxWorkers: 100,
			},
		},
		{
			in: []cluster.RetrieveOption{
				cluster.WithMaxWorkers(900),
				cluster.WithResultChannel(testChan),
				cluster.WithKeepChannelAlive(),
				cluster.WithWorkerPool(testWorkerPool),
			},
			exp: &cluster.RetrieveOptions{
				MaxWorkers:       900,
				ClustersChan:     testChan,
				KeepChannelAlive: true,
			},
		},
		{
			in: []cluster.RetrieveOption{
				cluster.WithResultChannel(nil),
			},
			expErr: cluster.ErrorInvalidChannel,
		},
		{
			in: []cluster.RetrieveOption{
				cluster.WithWorkerPool(nil),
			},
			expErr: cluster.ErrorInvalidWorkerPool,
		},
	}
	a := assert.New(t)

	for i, testCase := range cases {
		res, err := ParseClusterRetrieveOptions(testCase.in...)
		if testCase.expErr != nil {
			if !a.Equal(testCase.expErr, err) {
				a.FailNow(caseFailed(i))
				return
			}

			if !a.Nil(res) {
				a.FailNow(caseFailed(i))
				return
			}

			continue
		}

		a.Equal(testCase.expErr, err)
		a.Equal(testCase.exp.MaxWorkers, res.MaxWorkers)
		a.True(reflect.DeepEqual(testCase.exp.Regions, res.Regions))
		if testCase.exp.WorkerPool != nil {
			a.Equal(testCase.exp.WorkerPool, res.WorkerPool)
		}
		a.Equal(testCase.exp.ClustersChan, res.ClustersChan)
		a.Equal(testCase.exp.KeepChannelAlive, res.KeepChannelAlive)
	}
}

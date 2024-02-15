// Copyright (c) 2023 Cisco Systems, Inc. and its affiliates
// All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"strings"

	"github.com/panjf2000/ants"
)

type ClustersRetriever interface {
	Retrieve(ctx context.Context, opt ...RetrieveOption) (*RetrieveResults, error)
	GetCluster(ctx context.Context, location string, name string) (DiscoveredCluster, error)
}

type RetrieveOptions struct {
	Regions          map[string]bool
	ClustersChan     chan<- DiscoveredCluster
	KeepChannelAlive bool
	MaxWorkers       uint
	WorkerPool       *ants.Pool
}

type RetrieveOption func(opts *RetrieveOptions) error

// Get clusters running *exclusively* in the regions.
//
// This function will provide de-duplication automatically, but will *not*
// validate whether the regions are enabled for the cloud accout or if they
// actually exist.
func WithRegions(regions ...string) RetrieveOption {
	return func(opts *RetrieveOptions) error {
		if opts.Regions == nil {
			opts.Regions = map[string]bool{}
		}

		for _, region := range regions {
			_region := strings.Trim(region, " ")
			if _region != "-" {
				// "-" has a special meaning in GKE: it means "all regions".
				// If the user specified this along with other regions, then
				// we remove it because it would make it an error.
				opts.Regions[_region] = true
			}
		}

		return nil
	}
}

// Send results to this channel. The retriever will send clusters to this
// channel as soon as it finds them.
//
// Once done, the retriever will close this channel to signal that there is no
// more data to send.
//
// Make sure your channel is buffered before sending it to the retriever to
// avoid performance penalties in case you expect many clusters to be found.
func WithResultChannel(clustersChan chan<- DiscoveredCluster) RetrieveOption {
	return func(opts *RetrieveOptions) error {
		if clustersChan == nil {
			return ErrorInvalidChannel
		}

		opts.ClustersChan = clustersChan
		return nil
	}
}

func WithKeepChannelAlive() RetrieveOption {
	return func(opts *RetrieveOptions) error {
		opts.KeepChannelAlive = true
		return nil
	}
}

// Set the maximum number of workers, or concurrent tasks.
//
// By default, retrievers will try to maximize concurrency and prioritize
// non-blocking operations, especially in case many regions need to be
// queried. This option sets a limit on the concurrent tasks that can be
// performed.
//
// This option will have no effect in case you are providing you own worker
// pool, in which case you will have to limit concurrency on your own.
//
// Providing `0` will make it use the default value (100).
func WithMaxWorkers(workers uint) RetrieveOption {
	return func(opts *RetrieveOptions) error {
		if workers > 0 {
			opts.MaxWorkers = workers
		}
		return nil
	}
}

// Sets a custom worker pool.
//
// This is useful in case you are already using a worker pool for your own
// purposes and want to re-use your idle workers to perform these tasks.
func WithWorkerPool(workerPool *ants.Pool) RetrieveOption {
	return func(opts *RetrieveOptions) error {
		if workerPool == nil {
			return ErrorInvalidWorkerPool
		}

		opts.WorkerPool = workerPool
		return nil
	}
}

type RetrieveResults struct {
	DiscoveredClusters []DiscoveredCluster
	Errors             []RegionError
}

type RegionError struct {
	Region      string
	ClusterName *string
	Error       error
}

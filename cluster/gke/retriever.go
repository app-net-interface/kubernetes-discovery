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

package gke

import (
	"context"
	"errors"
	"path"

	container "cloud.google.com/go/container/apiv1"
	"cloud.google.com/go/container/apiv1/containerpb"
	"github.com/googleapis/gax-go"
	"github.com/panjf2000/ants"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	cluster "github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/app-net-interface/kubernetes-discovery/cluster/internal/utils"
)

const (
	projectPath           = "projects"
	locationsPath         = "locations"
	clustersPath          = "clusters"
	defaultWorkerPoolSize = 100
)

type ClustersRetriever struct {
	credentials   *google.Credentials
	clientOptions []option.ClientOption

	// Client injected for unit tests.
	// NOTE: on future, this way of testing may be modified.
	client clusterRetrieverInterfaceClient
}

type result struct {
	data   *GkeDiscoveredCluster
	done   bool
	region string
	err    error
}

type clusterRetrieverInterfaceClient interface {
	ListClusters(ctx context.Context, req *containerpb.ListClustersRequest, opts ...gax.CallOption) (*containerpb.ListClustersResponse, error)
	GetCluster(ctx context.Context, req *containerpb.GetClusterRequest, opts ...gax.CallOption) (*containerpb.Cluster, error)
	Close() error
}

func NewClustersRetriever(credentials *google.Credentials) (*ClustersRetriever, error) {
	if credentials == nil {
		return nil, cluster.ErrorInvalidCredentials
	}

	return &ClustersRetriever{
		credentials:   credentials,
		clientOptions: []option.ClientOption{option.WithCredentials(credentials)},
	}, nil
}

func (r *ClustersRetriever) Retrieve(ctx context.Context, opts ...cluster.RetrieveOption) (*cluster.RetrieveResults, error) {
	// -------------------------------------
	// Init
	// -------------------------------------

	options, err := utils.ParseClusterRetrieveOptions(opts...)
	if err != nil {
		return nil, err
	}

	pool, err := func() (*ants.Pool, error) {
		if options.WorkerPool != nil {
			return options.WorkerPool, nil
		}

		return ants.NewPool(defaultWorkerPoolSize)
	}()
	if err != nil {
		return nil, errors.Join(cluster.ErrorCreatingPool, err)
	}
	client, err := func() (clusterRetrieverInterfaceClient, error) {
		if r.client == nil {
			return container.NewClusterManagerClient(ctx, r.clientOptions...)
		}

		// We are testing
		return r.client, nil
	}()

	if err != nil {
		return nil, errors.Join(cluster.ErrorCannotGetClient, err)
	}
	chanResult := make(chan *result, 256)
	resp := &cluster.RetrieveResults{
		DiscoveredClusters: []cluster.DiscoveredCluster{},
		Errors:             []cluster.RegionError{},
	}
	wait := 0

	defer func() {
		close(chanResult)
		client.Close()

		// Release the pool, but only if this is *our* pool, not the user's.
		if options.WorkerPool == nil {
			pool.Release()
		}

		// Close the channels or send completion.
		if options.ClustersChan != nil && !options.KeepChannelAlive {
			close(options.ClustersChan)
		}
	}()

	// -------------------------------------
	// Start task(s)
	// -------------------------------------

	if len(options.Regions) == 0 {
		// "-" means "all regions" in GCP
		options.Regions = map[string]bool{"-": true}
	}

	for region := range options.Regions {
		wait++
		err := pool.Submit(func(regionName string) func() {
			return func() {
				r.getClustersFromRegion(ctx, client, regionName, chanResult)
			}
		}(region))
		if err != nil {
			wait--
			resp.Errors = append(resp.Errors, cluster.RegionError{
				Region: region,
				Error:  errors.Join(cluster.ErrorCannotSubmitTask, err),
			})
		}
	}

	// -------------------------------------
	// Wait for results (and send them)
	// -------------------------------------

	for wait > 0 {
		select {
		case <-ctx.Done():
			// We will wait for all workers to send a result.done
		case res := <-chanResult:
			switch {

			case res.err != nil:
				resp.Errors = append(resp.Errors, cluster.RegionError{
					Region: func() string {
						if res.region != "-" {
							return res.region
						}

						return ""
					}(),
					Error: res.err,
				})

			case res.done:
				wait--

			case res.data != nil:
				if options.ClustersChan != nil {
					options.ClustersChan <- res.data
				}

				resp.DiscoveredClusters = append(resp.DiscoveredClusters, res.data)
			}
		}
	}

	return resp, nil
}

func (r *ClustersRetriever) getClustersFromRegion(ctx context.Context, client clusterRetrieverInterfaceClient, regionName string, resultsChan chan<- *result) {
	defer func() {
		resultsChan <- &result{
			region: regionName,
			done:   true,
		}
	}()

	resp, err := client.ListClusters(ctx, &containerpb.ListClustersRequest{
		Parent: path.Join(
			projectPath,
			r.credentials.ProjectID,
			locationsPath,
			regionName,
		),
	})
	if err != nil {
		resultsChan <- &result{
			region: regionName,
			err:    err,
		}
		return
	}

	for _, foundCluster := range resp.Clusters {
		resultsChan <- &result{
			region: regionName,
			data:   castGkeCluster(foundCluster, r.credentials),
		}
	}
}

func (r *ClustersRetriever) GetCluster(ctx context.Context, region, name string) (cluster.DiscoveredCluster, error) {
	if name == "" {
		return nil, cluster.ErrorInvalidClusterName
	}
	if region == "" {
		return nil, cluster.ErrorInvalidRegionName
	}

	client, err := func() (clusterRetrieverInterfaceClient, error) {
		if r.client == nil {
			return container.NewClusterManagerClient(ctx,
				option.WithCredentials(r.credentials))
		}

		return r.client, nil
	}()
	if err != nil {
		return nil, errors.Join(cluster.ErrorCannotGetClient, err)
	}

	resp, err := client.GetCluster(ctx, &containerpb.GetClusterRequest{
		Name: path.Join(projectPath, r.credentials.ProjectID, locationsPath, region, clustersPath, name),
	})
	if err != nil {
		return nil, err
	}

	return castGkeCluster(resp, r.credentials), nil
}

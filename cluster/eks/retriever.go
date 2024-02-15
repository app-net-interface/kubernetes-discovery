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

package eks

import (
	"context"
	"errors"
	"sync"

	cluster "github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/app-net-interface/kubernetes-discovery/cluster/token"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/panjf2000/ants"
)

type ClustersRetriever struct {
	cfg aws.Config
}

type eksClient interface {
	eks.ListClustersAPIClient
	eks.DescribeClusterAPIClient
}

func NewClustersRetriever(cfg aws.Config) *ClustersRetriever {
	return &ClustersRetriever{
		cfg: cfg,
	}
}

func (r *ClustersRetriever) Retrieve(ctx context.Context, opts ...cluster.RetrieveOption) (*cluster.RetrieveResults, error) {
	// -------------------------------------
	// Parse options
	// -------------------------------------

	options := &cluster.RetrieveOptions{}
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, err
		}
	}

	if options.MaxWorkers == 0 {
		options.MaxWorkers = 100
	}

	pool := options.WorkerPool
	if options.WorkerPool == nil {
		wp, err := ants.NewPool(int(options.MaxWorkers))
		if err != nil {
			return nil, errors.Join(cluster.ErrorCreatingPool, err)
		}

		pool = wp
	}

	// -------------------------------------
	// Init data
	// -------------------------------------

	chanResult := make(chan *result, 256)
	resp := &cluster.RetrieveResults{
		DiscoveredClusters: []cluster.DiscoveredCluster{},
		Errors:             []cluster.RegionError{},
	}
	wg := sync.WaitGroup{}
	wait := 0

	defer func() {
		wg.Wait()
		close(chanResult)

		// Release the pool, but only if this is *our* pool, not user's.
		if options.WorkerPool == nil {
			pool.Release()
		}

		// Close the channels or send completion.
		if options.ClustersChan != nil && !options.KeepChannelAlive {
			close(options.ClustersChan)
		}
	}()

	ec2Client := ec2.NewFromConfig(r.cfg)
	regions, err := ec2Client.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, errors.Join(cluster.ErrorCannotGetRegions, errors.Join(cluster.ErrorCannotGetRegions, err))
	}

	// -------------------------------------
	// Start task(s)
	// -------------------------------------

	for i := 0; i < len(regions.Regions); i++ {
		regionName := *regions.Regions[i].RegionName
		allowed := false

		if len(options.Regions) == 0 {
			allowed = true
		} else {
			if _, exists := options.Regions[regionName]; exists {
				allowed = true
			}
		}

		if !allowed {
			continue
		}

		wait++
		wg.Add(1)
		err := pool.Submit(func(reg string) func() {
			return func() {
				defer wg.Done()
				client := eks.NewFromConfig(r.cfg, func(o *eks.Options) {
					o.Region = reg
				})
				r.getClustersInRegion(ctx, client, pool, reg, chanResult)
			}
		}(regionName))
		if err != nil {
			wait--
			resp.Errors = append(resp.Errors, cluster.RegionError{
				Region: regionName,
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
			wait = 0 // graceful stop
		case res := <-chanResult:
			switch {

			case res.err != nil:
				resp.Errors = append(resp.Errors, cluster.RegionError{
					Region: res.region,
					Error:  res.err,
				})

				if options.ClustersChan != nil {
					options.ClustersChan <- &EksDiscoveredCluster{
						err: err,
					}
				}

			case res.done:
				wait--

			default:
				if options.ClustersChan != nil {
					options.ClustersChan <- res.data
				}

				resp.DiscoveredClusters = append(resp.DiscoveredClusters, res.data)
			}
		}
	}

	return resp, nil
}

func (r *ClustersRetriever) getClustersInRegion(ctx context.Context, client eksClient, pool *ants.Pool, regionName string, resChan chan<- *result) {
	wg := sync.WaitGroup{}
	var nextToken *string

	defer func() {
		wg.Wait()
		resChan <- &result{
			region: regionName,
			done:   true,
		}
	}()

	for {
		resp, err := client.ListClusters(ctx, &eks.ListClustersInput{
			NextToken: nextToken,
		})
		if err != nil {
			resChan <- &result{
				region: regionName,
				err:    err,
			}

			return
		}

		for _, cl := range resp.Clusters {
			if ctx.Err() != nil {
				break
			}

			wg.Add(1)
			err := pool.Submit(func(clusterName string) func() {
				return func() {
					defer wg.Done()
					r.retrieveSingleCluster(ctx, client, clusterName, regionName, resChan)
				}
			}(cl))
			if err != nil {
				resChan <- &result{
					region: regionName,
					err:    errors.Join(cluster.ErrorCannotSubmitTask, err),
				}
			}
		}

		if resp.NextToken == nil {
			break
		}

		nextToken = resp.NextToken
	}
}

func (r *ClustersRetriever) GetCluster(ctx context.Context, region, name string) (cluster.DiscoveredCluster, error) {
	if name == "" {
		return nil, cluster.ErrorInvalidClusterName
	}
	if region == "" {
		return nil, cluster.ErrorInvalidRegionName
	}

	resChan := make(chan *result, 2)
	client := eks.NewFromConfig(r.cfg, func(o *eks.Options) {
		o.Region = region
	})
	r.retrieveSingleCluster(ctx, client, name, region, resChan)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case val := <-resChan:
			switch {
			case val.err != nil:
				return nil, val.err
			default:
				// The next one would be a .done value, so we just return here.
				return val.data, nil
			}
		}
	}
}

func (r *ClustersRetriever) retrieveSingleCluster(ctx context.Context, client eksClient, clusterName, regionName string, resChan chan<- *result) {
	resp, err := client.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: &clusterName,
	})
	if err != nil {
		resChan <- &result{
			region: regionName,
			err:    err,
		}
		return
	}

	cl := resp.Cluster

	if cl.Name == nil {
		// Some time EKS will throw nil values, especially when the cluster is
		// in creation state.
		// So, if *at least* the name is not there, then we just return an error.
		resChan <- &result{
			region: regionName,
			err:    cluster.ErrorClusterNotReady,
		}
		return
	}

	status, statusOther := castEKSStatus(cl.Status)
	resChan <- &result{
		region: regionName,
		data: &EksDiscoveredCluster{
			data: &cluster.Cluster{
				Name:              aws.ToString(cl.Name),
				ID:                aws.ToString(cl.Arn),
				EndpointAddress:   aws.ToString(cl.Endpoint),
				CaCertificateData: aws.ToString(cl.CertificateAuthority.Data),
				Platform:          cluster.EKS,
				Location:          regionName,
				CreatedAt:         *resp.Cluster.CreatedAt,
				Status:            status,
				StatusOther:       statusOther,
				OriginalObject:    cl,
			},
			tokenAuth: token.NewAwsTokenGenerator(*cl.Name, func() aws.Config {
				newCfg := r.cfg
				newCfg.Region = regionName
				return newCfg
			}()),
		},
	}
}

type result struct {
	data   *EksDiscoveredCluster
	done   bool
	region string
	err    error
}

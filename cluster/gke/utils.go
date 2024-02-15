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
	"time"

	"cloud.google.com/go/container/apiv1/containerpb"
	cluster "github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/app-net-interface/kubernetes-discovery/cluster/token"
	"golang.org/x/oauth2/google"
)

func castGKEStatus(status containerpb.Cluster_Status) (cluster.Status, *string) {
	switch status {
	case containerpb.Cluster_PROVISIONING:
		return cluster.Creating, nil
	case containerpb.Cluster_STOPPING:
		return cluster.Stopping, nil
	case containerpb.Cluster_RUNNING:
		return cluster.Active, nil
	case containerpb.Cluster_ERROR:
		return cluster.Inactive, nil
	case containerpb.Cluster_RECONCILING:
		return cluster.Updating, nil
	case containerpb.Cluster_STATUS_UNSPECIFIED,
		containerpb.Cluster_DEGRADED,
		-1:
		return cluster.Deleted, nil
	default:
		originalStatus := status.String()
		return cluster.Other, &originalStatus
	}
}

func castGkeCluster(cl *containerpb.Cluster, creds *google.Credentials) *GkeDiscoveredCluster {
	status, statusOther := castGKEStatus(cl.Status)
	return &GkeDiscoveredCluster{
		data: &cluster.Cluster{
			Name:              cl.Name,
			ID:                cl.Id,
			Platform:          cluster.GKE,
			EndpointAddress:   cl.Endpoint,
			CaCertificateData: cl.MasterAuth.ClusterCaCertificate,
			Location:          cl.Location,
			CreatedAt: func() time.Time {
				val, _ := time.Parse(time.RFC3339, cl.CreateTime)
				return val
			}(),
			Status:         status,
			StatusOther:    statusOther,
			OriginalObject: cl,
		},
		tokenAuth: token.NewGcpTokenGenerator(creds),
	}
}

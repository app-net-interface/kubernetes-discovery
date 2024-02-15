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

package aks

import (
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/app-net-interface/kubernetes-discovery/cluster"
	kc "github.com/volvo-cars/lingon/pkg/kubeconfig"
)

func castStatus(code *armcontainerservice.Code) (cluster.Status, *string) {
	switch *code {
	case armcontainerservice.CodeRunning:
		return cluster.Active, nil
	case armcontainerservice.CodeStopped:
		return cluster.Inactive, nil
	default:
		return cluster.Other, nil
	}
}

func buildClusterData(data *armcontainerservice.ManagedCluster, auth *kc.Config) *cluster.Cluster {
	status, originalStatus := castStatus(data.Properties.PowerState.Code)
	return &cluster.Cluster{
		Name:              *data.Name,
		ID:                *data.ID,
		EndpointAddress:   auth.Clusters[0].Cluster.Server,
		CaCertificateData: auth.Clusters[0].Cluster.CertificateAuthorityData,
		Platform:          cluster.AKS,
		Location:          *data.Location,
		CreatedAt: func() time.Time {
			if data.SystemData != nil {
				return *data.SystemData.CreatedAt
			}

			return time.Now()
		}(),
		Status:         status,
		StatusOther:    originalStatus,
		OriginalObject: data,
	}
}

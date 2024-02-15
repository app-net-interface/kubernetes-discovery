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
	cluster "github.com/app-net-interface/kubernetes-discovery/cluster"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
)

func castEKSStatus(status ekstypes.ClusterStatus) (cluster.Status, *string) {
	switch status {
	case ekstypes.ClusterStatusCreating:
		return cluster.Creating, nil
	case ekstypes.ClusterStatusDeleting:
		return cluster.Stopping, nil
	case ekstypes.ClusterStatusActive:
		return cluster.Active, nil
	case ekstypes.ClusterStatusPending:
		return cluster.Inactive, nil
	case ekstypes.ClusterStatusUpdating:
		return cluster.Updating, nil
	case "":
		return cluster.Deleted, nil
	default:
		originalStatus := string(status)
		return cluster.Other, &originalStatus
	}
}

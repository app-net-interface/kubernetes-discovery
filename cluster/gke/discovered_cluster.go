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
	"encoding/base64"

	"github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/app-net-interface/kubernetes-discovery/cluster/token"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type GkeDiscoveredCluster struct {
	data      *cluster.Cluster
	err       error
	tokenAuth *token.GcpTokenGenerator
}

func (g *GkeDiscoveredCluster) GetData() (*cluster.Cluster, error) {
	if g.err != nil {
		return nil, g.err
	}

	return g.data, nil
}

func (g *GkeDiscoveredCluster) GetToken(ctx context.Context) (*token.Token, error) {
	if g.err != nil {
		return nil, g.err
	}

	return g.tokenAuth.Get(ctx)
}

func (g *GkeDiscoveredCluster) GetConfig(ctx context.Context) (*rest.Config, error) {
	token, err := g.GetToken(ctx)
	if err != nil {
		return nil, err
	}

	ca, err := base64.StdEncoding.DecodeString(g.data.CaCertificateData)
	if err != nil {
		return nil, err
	}
	return &rest.Config{
		Host:        g.data.EndpointAddress,
		BearerToken: token.Value,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: ca,
		},
	}, nil
}

func (g *GkeDiscoveredCluster) GetClientset(ctx context.Context) (*kubernetes.Clientset, error) {
	config, err := g.GetConfig(ctx)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

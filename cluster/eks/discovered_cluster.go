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
	"encoding/base64"

	"github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/app-net-interface/kubernetes-discovery/cluster/token"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type EksDiscoveredCluster struct {
	data      *cluster.Cluster
	err       error
	tokenAuth *token.AwsTokenGenerator
}

func (e *EksDiscoveredCluster) GetData() (*cluster.Cluster, error) {
	if e.err != nil {
		return nil, e.err
	}

	return e.data, nil
}

func (e *EksDiscoveredCluster) GetToken(ctx context.Context) (*token.Token, error) {
	if e.err != nil {
		return nil, e.err
	}

	return e.tokenAuth.Get(ctx)
}

func (e *EksDiscoveredCluster) GetConfig(ctx context.Context) (*rest.Config, error) {
	token, err := e.GetToken(ctx)
	if err != nil {
		return nil, err
	}

	ca, err := base64.StdEncoding.DecodeString(e.data.CaCertificateData)
	if err != nil {
		return nil, err
	}

	return &rest.Config{
		Host:        e.data.EndpointAddress,
		BearerToken: token.Value,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: ca,
		},
	}, nil
}

func (e *EksDiscoveredCluster) GetClientset(ctx context.Context) (*kubernetes.Clientset, error) {
	config, err := e.GetConfig(ctx)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

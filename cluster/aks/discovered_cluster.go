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
	"context"
	"encoding/base64"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4"
	"github.com/app-net-interface/kubernetes-discovery/cluster"
	"github.com/app-net-interface/kubernetes-discovery/cluster/token"
	kc "github.com/volvo-cars/lingon/pkg/kubeconfig"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type AksDiscoveredCluster struct {
	data           *cluster.Cluster
	err            error
	identity       azcore.TokenCredential
	subscriptionID string
	resourceGroup  string
}

func (a *AksDiscoveredCluster) GetData() (*cluster.Cluster, error) {
	if a.err != nil {
		return nil, a.err
	}

	return a.data, nil
}

func (a *AksDiscoveredCluster) getConf(ctx context.Context) (*kc.Config, error) {
	cl, ok := a.data.OriginalObject.(*armcontainerservice.ManagedCluster)
	if !ok {
		return nil, fmt.Errorf("cluster data is invalid")
	}

	switch {
	case cl.ID == nil:
	case cl.ID != nil && *cl.ID == "":
		return nil, fmt.Errorf("invalid cluster ID")
	case cl.Name == nil:
	case cl.Name != nil && *cl.Name == "":
		return nil, fmt.Errorf("invalid cluster name")
	}

	clientFactory, err := armcontainerservice.NewClientFactory(a.subscriptionID, a.identity, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot get client factory: %w", err)
	}
	mclient := clientFactory.NewManagedClustersClient()
	admCreds, err := mclient.ListClusterAdminCredentials(ctx, a.resourceGroup, *cl.Name, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot get credentials: %w", err)
	}

	if len(admCreds.Kubeconfigs) == 0 {
		return nil, fmt.Errorf("no credentials found for cluster")
	}

	conf := kc.New()
	if err := conf.Unmarshal(admCreds.Kubeconfigs[0].Value); err != nil {
		return nil, fmt.Errorf("cannot get config for cluster: %w", err)
	}

	return conf, nil
}

func (a *AksDiscoveredCluster) GetToken(ctx context.Context) (*token.Token, error) {
	if a.err != nil {
		return nil, a.err
	}

	conf, err := a.getConf(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot get cluster configuration: %w", err)
	}

	return &token.Token{
		Value: conf.Users[0].User.Token,
		// Expiration: , TODO: this will be tested.
	}, nil
}

func (a *AksDiscoveredCluster) GetConfig(ctx context.Context) (*rest.Config, error) {
	token, err := a.GetToken(ctx)
	if err != nil {
		return nil, a.err
	}

	ca, err := base64.StdEncoding.DecodeString(a.data.CaCertificateData)
	if err != nil {
		return nil, err
	}
	return &rest.Config{
		Host:        a.data.EndpointAddress,
		BearerToken: token.Value, // TODO: maybe use certificates?
		TLSClientConfig: rest.TLSClientConfig{
			CAData: ca,
		},
	}, nil
}

func (a *AksDiscoveredCluster) GetClientset(ctx context.Context) (*kubernetes.Clientset, error) {
	config, err := a.GetConfig(ctx)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

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

package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/panjf2000/ants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type GetResults struct {
	Services []*Service
	Errors   []error
}

func GetServices(ctx context.Context, clientset kubernetes.Interface, opts ...RetrieveOption) (*GetResults, error) {
	// -------------------------------------
	// Parse options
	// -------------------------------------

	options := &RetrieveOptions{}
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, errors.Join(ErrorInvalidOptions, err)
		}
	}

	if len(options.Namespaces) == 0 {
		options.Namespaces = map[string]bool{"": true}
	}

	if options.MaxWorkers == 0 {
		options.MaxWorkers = 100
	}

	pool := options.WorkerPool
	if options.WorkerPool == nil {
		_pool, err := ants.NewPool(int(options.MaxWorkers))
		if err != nil {
			return nil, errors.Join(ErrorCreatingPool, err)
		}

		pool = _pool
	}

	// -------------------------------------
	// Init data
	// -------------------------------------

	nodesAddresses := []v1.NodeAddress{}
	chanResult := make(chan *result, 256)
	wait := 0
	wg := sync.WaitGroup{}
	resp := &GetResults{
		Services: []*Service{},
		Errors:   []error{},
	}

	if options.NodePorts {
		err := func() error {
			nodesList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			if err != nil {
				return err
			}

			for _, node := range nodesList.Items {
				nodesAddresses = append(nodesAddresses, node.Status.Addresses...)
			}

			return nil
		}()

		if err != nil {
			resp.Errors = append(resp.Errors,
				fmt.Errorf("could not get node IPs: %w", err))
		}
	}

	defer func() {
		wg.Wait()
		close(chanResult)

		if options.WorkerPool == nil {
			pool.Release()
		}

		if options.ServicesChan != nil {
			if options.CompletionChan != nil {
				options.CompletionChan <- struct{}{}
			} else {
				close(options.ServicesChan)
			}
		}
	}()

	// -------------------------------------
	// Start task(s)
	// -------------------------------------

	for namespace := range options.Namespaces {
		wg.Add(1)
		wait++
		if err := pool.Submit(func(ns string) func() {
			return func() {
				defer wg.Done()
				getServicesInNamespace(ctx, clientset, options, nodesAddresses, ns, chanResult)
			}

		}(namespace)); err != nil {
			wg.Done()
			wait--
			resp.Errors = append(resp.Errors, errors.Join(ErrorCannotSubmitTask, err))
		}
	}

	// -------------------------------------
	// Wait for results (and send them)
	// -------------------------------------

	for wait > 0 {
		select {
		case <-ctx.Done():
			wait = 0 // Graceful termination
		case res := <-chanResult:
			switch {
			case res.err != nil:
				resp.Errors = append(resp.Errors, res.err)
			case res.done:
				wait--
			default:
				resp.Services = append(resp.Services, res.data)

				if options.ServicesChan != nil {
					options.ServicesChan <- res.data
				}
			}
		}
	}

	return resp, nil
}

func getServicesInNamespace(ctx context.Context, clientset kubernetes.Interface, opts *RetrieveOptions, nodesAddresses []v1.NodeAddress, namespace string, chanResult chan<- *result) {
	defer func() {
		chanResult <- &result{
			done:      true,
			namespace: namespace,
		}
	}()

	// TODO: in future the continue feature will be implemented
	servicesList, err := clientset.
		CoreV1().
		Services(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		chanResult <- &result{
			err: err,
		}

		return
	}

	for _, service := range servicesList.Items {

		serv := func() *Service {
			parsedService := Service{
				Name:           service.Name,
				Namespace:      service.Namespace,
				Type:           service.Spec.Type,
				Ports:          castPorts(&service),
				OriginalObject: service,
			}

			// TODO: on future this will have to understand if it is an internal or
			// external load balancer
			switch service.Spec.Type {

			case v1.ServiceTypeLoadBalancer:
				for _, hostName := range service.Status.LoadBalancer.Ingress {
					parsedService.HostNames =
						append(parsedService.HostNames, hostName.Hostname)
				}
				for _, addr := range service.Status.LoadBalancer.Ingress {
					parsedService.IPs = append(parsedService.IPs, addr.IP)
				}

			case v1.ServiceTypeExternalName:
				if !opts.ExternalNames {
					return nil
				}
				parsedService.HostNames = []string{service.Spec.ExternalName}

			case v1.ServiceTypeNodePort:
				if !opts.NodePorts {
					return nil
				}

				for _, nodeAddr := range nodesAddresses {
					switch nodeAddr.Type {
					case v1.NodeExternalDNS:
						parsedService.HostNames =
							append(parsedService.HostNames, nodeAddr.Address)
					case v1.NodeExternalIP:
						parsedService.IPs =
							append(parsedService.IPs, nodeAddr.Address)
					}
				}

			default:
				return nil
			}

			return &parsedService
		}()

		if serv != nil {
			chanResult <- &result{
				data:      serv,
				namespace: namespace,
			}
		}
	}
}

type result struct {
	data      *Service
	done      bool
	namespace string
	err       error
}

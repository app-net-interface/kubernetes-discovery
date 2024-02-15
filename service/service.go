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
	"github.com/panjf2000/ants"
	v1 "k8s.io/api/core/v1"
)

type Service struct {
	Name           string
	Namespace      string
	Type           v1.ServiceType
	HostNames      []string
	IPs            []string
	Ports          []uint32
	OriginalObject v1.Service
}

type LoadBalancerType int

const (
	AllLoadBalancers = iota
	InternalLoadBalancers
	ExternalLoadBalancers
)

type RetrieveOptions struct {
	NodePorts        bool
	ExternalNames    bool
	Namespaces       map[string]bool
	LoadBalancerType LoadBalancerType
	Labels           map[string]string
	MaxWorkers       uint
	WorkerPool       *ants.Pool
	ServicesChan     chan<- *Service
	CompletionChan   chan<- struct{}
}

type RetrieveOption func(*RetrieveOptions) error

func WithNodePorts() RetrieveOption {
	return func(ro *RetrieveOptions) error {
		ro.NodePorts = true
		return nil
	}
}

func WithExternalNames() RetrieveOption {
	return func(ro *RetrieveOptions) error {
		ro.ExternalNames = true
		return nil
	}
}

func WithInternalLoadBalancers() RetrieveOption {
	return func(ro *RetrieveOptions) error {
		ro.LoadBalancerType = InternalLoadBalancers
		return nil
	}
}

func WithExternalLoadBalancers() RetrieveOption {
	return func(ro *RetrieveOptions) error {
		ro.LoadBalancerType = ExternalLoadBalancers
		return nil
	}
}

func WithNamespaces(namespaces ...string) RetrieveOption {
	return func(ro *RetrieveOptions) error {
		if ro.Namespaces == nil {
			ro.Namespaces = map[string]bool{}
		}

		for _, ns := range namespaces {
			if ns != "" {
				ro.Namespaces[ns] = true
			}
		}

		return nil
	}
}

func WithLabel(key, value string) RetrieveOption {
	return func(ro *RetrieveOptions) error {
		if ro.Labels == nil {
			ro.Labels = map[string]string{}
		}

		ro.Labels[key] = value
		return nil
	}
}

func WithLabels(labels map[string]string) RetrieveOption {
	return func(ro *RetrieveOptions) error {
		ro.Labels = labels
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
		if opts.MaxWorkers > 0 {
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

// Send results to this channel. The retriever will send clusters to this
// channel as soon as it finds them.
//
// Once done, the retriever will close this channel to signal that there is no
// more data to send.
//
// Make sure your channel is buffered before sending it to the retriever to
// avoid performance penalties in case you expect many clusters to be found.
func WithResultChannel(servicesChan chan<- *Service) RetrieveOption {
	return func(opts *RetrieveOptions) error {
		if servicesChan == nil {
			return ErrorInvalidChannel
		}

		opts.ServicesChan = servicesChan
		return nil
	}
}

// Signal completion to this channel.
//
// By default, the retriever will close the channel you provide via
// `WithResultChannel` when its task is complete.
// This option will override this behavior by keeping that channel alive, and
// instead signal completion to this other channel by sending an empty array.
// This is useful in case you are re-using the channel for other purposes or
// when you are looping through `select`.
//
// If not used in conjunction to `WithResultChannel`, this option will have
// no effect.
func WithCompletionChannel(completionChan chan<- struct{}) RetrieveOption {
	return func(opts *RetrieveOptions) error {
		opts.CompletionChan = completionChan
		return nil
	}
}

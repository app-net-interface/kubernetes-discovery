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

package cluster

import "errors"

var (
	ErrorInvalidChannel        error = errors.New("invalid channel provided")
	ErrorInvalidCredentials    error = errors.New("invalid credentials provided")
	ErrorInvalidOptions        error = errors.New("invalid options provided")
	ErrorInvalidWorkerPool     error = errors.New("invalid worker pool")
	ErrorCreatingPool          error = errors.New("cannot create worker pool")
	ErrorNoCredentialsProvided error = errors.New("no credentials provided")
	ErrorCannotSubmitTask      error = errors.New("cannot submit task to worker pool")
	ErrorCannotGetClient       error = errors.New("cannot get client")
	ErrorCannotGetRegions      error = errors.New("cannot get regions list")
	ErrorInvalidClusterName    error = errors.New("invalid cluster name provided")
	ErrorInvalidRegionName     error = errors.New("invalid region name provided")
	ErrorClusterNotReady       error = errors.New("cluster is not ready")
)

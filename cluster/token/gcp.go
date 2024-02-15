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

package token

import (
	"context"

	"golang.org/x/oauth2/google"
)

type GcpTokenGenerator struct {
	creds *google.Credentials
}

func NewGcpTokenGenerator(creds *google.Credentials) *GcpTokenGenerator {
	return &GcpTokenGenerator{creds}
}

func (g *GcpTokenGenerator) Get(ctx context.Context) (*Token, error) {
	token, err := g.creds.TokenSource.Token()
	if err != nil {
		return nil, err
	}

	return &Token{
		Value:      token.AccessToken,
		Expiration: token.Expiry,
	}, nil
}

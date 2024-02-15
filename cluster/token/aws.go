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
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type AwsTokenGenerator struct {
	clusterID string
	cfg       aws.Config
}

func NewAwsTokenGenerator(clusterID string, cfg aws.Config) *AwsTokenGenerator {
	return &AwsTokenGenerator{
		clusterID: clusterID,
		cfg:       cfg,
	}
}

func (a *AwsTokenGenerator) Get(ctx context.Context) (*Token, error) {
	stsClient := sts.NewFromConfig(a.cfg)
	preSignClient := sts.NewPresignClient(stsClient)
	out, err := preSignClient.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{}, func(opt *sts.PresignOptions) {
		opt.Presigner = &customHTTPPresignerV4{clusterID: a.clusterID, client: opt.Presigner}
	})
	if err != nil {
		return nil, err
	}

	return &Token{
		Value: fmt.Sprintf("%s.%s",
			"k8s-aws-v1",
			base64.RawURLEncoding.EncodeToString([]byte(out.URL))),
		Expiration: time.Now().Add(15 * time.Minute),
	}, nil
}

type customHTTPPresignerV4 struct {
	clusterID string
	client    sts.HTTPPresignerV4
}

func (p *customHTTPPresignerV4) PresignHTTP(
	ctx context.Context,
	credentials aws.Credentials,
	r *http.Request,
	payloadHash string,
	service string,
	region string,
	signingTime time.Time,
	optFns ...func(*v4.SignerOptions),
) (url string, signedHeader http.Header, err error) {
	r.Header.Add("x-k8s-aws-id", p.clusterID)
	// NOTE: this value should ask Amazon to make the token last at least
	// 60 minutes, though we detected that maximum is still 20 minutes
	// regardless of what you write here. Nonetheless, we keep 60 in case
	// updates will be there.
	r.Header.Add("X-Amz-Expires", "60")
	return p.client.PresignHTTP(ctx, credentials, r, payloadHash, service, region, signingTime, optFns...)
}

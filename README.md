# Kubernetes Discovery

A library for discovering Kubernetes clusters from various Clouds and the
services inside them.

## Quickstart

### Retrieve clusters

Get a client:

First, import the library in your project, along with the libraries for your
chosen Kubernetes managed service -- for example `EKS`:

```go
import (
    cluster "github.com/app-net-interface/kubernetes-discovery/cluster"
    eks "github.com/app-net-interface/kubernetes-discovery/cluster/eks"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/credentials"
)
```

Then create a configuration for your chose cluster retriever:

```go
cfg, err := config.LoadDefaultConfig(ctx,
  config.WithRegion("us-west-2"),
  config.WithDefaultRegion("us-west-2"),
  // Provide credentials here
)

if err != nil {
  return nil, fmt.Errorf("cannot get AWS configuration: %w", err)
}
```

Now get the clusters retriever:

```go
retriever := eks.NewClustersRetriever(cfg)

clusters, err := retriever.Retrieve(
    context.TODO(), cluster.WithRegions("us-west-2", "us-west-1"))
if err != nil {
  log.Err(err).Msg("cannot get clusters")
  return
}

for _, foundCluster := range clusters.DiscoveredClusters {
  data, err := foundCluster.GetData()

  // Check the error and use data
}
```

### Retrieve services

Get the Kubernetes clientset:

```go
kclient, err := cl.GetClientset(context.TODO())
if err != nil {
  log.
    Err(err).
    Msg("cannot get Kubernetes clientset for cluster")
  return
}
```

Now retrieve services:

```go
servs, err := service.GetServices(ctx, kclient,
  service.WithExternalNames(),
  service.WithNodePorts(),
  service.WithNamespaces(namespaces...))
if err != nil {
  log.
    Err(err).
    Str("operation", "get-services").
    Msg("cannot get services")
  return
}

for _, serv := range servs.Services {
  // do something with the service
}
```

## Documentation

Read the full documentation about the usage, the supported clouds and functions
at [pkg.go](https://pkg.go.dev/app-net-interface/kubernetes-discovery)

## Contributing

Thank you for interest in contributing! Please refer to our
[contributing guide](CONTRIBUTING.md).

## License

kubernetes-discovery is released under the Apache 2.0 license. See
[LICENSE](./LICENSE).

kubernetes-discovery is also made possible thanks to
[third party open source projects](NOTICE).

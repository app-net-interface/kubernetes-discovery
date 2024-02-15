module github.com/app-net-interface/kubernetes-discovery

go 1.21.6

replace (
	github.com/app-net-interface/kubernetes-discovery/cluster => ./cluster
	github.com/app-net-interface/kubernetes-discovery/service => ./service
)

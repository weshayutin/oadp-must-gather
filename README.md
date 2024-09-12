# oadp-must-gather

refactor of OADP's must-gather

```shell
# fast test of must gather
go run cmd/main.go

# real test of must-gather
podman build -t ttl.sh/oadp/must-gather-$(git rev-parse --short HEAD)-$(echo $RANDOM) -f Dockerfile . --platform=<cluster-architecture>
podman push <this-image>
oc adm must-gather --image=<this-image> -- /usr/bin/gather -h
oc adm must-gather --image=<this-image>
# TODO test omg https://github.com/openshift/oadp-operator/pull/1269

# lint
GOBIN=$(pwd)/bin go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.56.2
./bin/golangci-lint run --fix
```

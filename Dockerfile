FROM quay.io/konveyor/builder:ubi9-latest AS builder
ARG TARGETOS

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY cmd/main.go cmd/main.go
COPY pkg/ pkg/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} go build -mod=mod -a -o gather cmd/main.go

FROM registry.access.redhat.com/ubi9-minimal:latest

# od adm must-gather uses this packages to download the output
RUN microdnf -y install rsync tar

COPY --from=builder /workspace/gather /usr/bin/gather

ENTRYPOINT ["/usr/bin/gather"]

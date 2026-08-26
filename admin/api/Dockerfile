# syntax=docker/dockerfile:1
#
# The admin API image. The build context is this module directory, which is
# self-contained: there is no Go workspace to reach out of.
#
#   docker build -t admin-api:dev admin/api

FROM golang:1.27-alpine AS build
WORKDIR /src

# Manifests first, so a source-only change reuses the dependency layer. go.sum is
# matched as an optional glob because it does not exist until the module takes its
# first dependency, and COPY fails on a name that matches nothing.
COPY go.mod go.su[m] ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/admin-api .

# Distroless because the API has no reason to hold a shell: it opens a socket and
# talks to three services. `nonroot` matches the chart's securityContext, which
# runs the pod as an unprivileged user with a read-only root filesystem.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/admin-api /admin-api
EXPOSE 8090
USER nonroot:nonroot
ENTRYPOINT ["/admin-api"]

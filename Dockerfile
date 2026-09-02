# syntax=docker/dockerfile:1.12

FROM golang:1.27.0@sha256:4013ae0f9e7994f8535c58c811f8f863fbed38b72e0d51e6592156f758d66146 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Each binary is built by its own stage so that building one image does not
# compile the other. The migration job and the API are deployed at different
# moments and must be separable, but they are built from one tree: an image pair
# assembled from two commits can apply a schema the running code does not expect.
FROM build AS build-api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM build AS build-migrate
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# The migration job runs to completion before the API is deployed and exits with
# a non-zero status when a migration fails, which is what stops the rollout. It
# carries the migrations embedded in the binary and serves no traffic.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS migrate

COPY --from=build-migrate /out/migrate /migrate

USER nonroot:nonroot
ENTRYPOINT ["/migrate"]

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS api

COPY --from=build-api /out/api /api

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]

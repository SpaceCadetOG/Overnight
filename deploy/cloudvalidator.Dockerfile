FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /cloudvalidator ./cmd/cloudvalidator

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates zstd && rm -rf /var/lib/apt/lists/*
COPY --from=build /cloudvalidator /usr/local/bin/cloudvalidator
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/cloudvalidator"]

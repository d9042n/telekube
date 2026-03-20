# Stage 1: Build
FROM golang:1.25-alpine AS builder
ARG VERSION=dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X github.com/d9042n/telekube/pkg/version.Version=${VERSION}" \
    -o /telekube ./cmd/telekube

# Stage 2: Runtime
FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata \
    && mkdir -p /home/telekube/.kube /etc/telekube \
    && chown -R 65534:65534 /home/telekube /etc/telekube
COPY --from=builder /telekube /usr/local/bin/telekube
USER 65534:65534
ENV HOME=/home/telekube
ENTRYPOINT ["telekube"]

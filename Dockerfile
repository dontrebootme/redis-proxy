FROM golang:alpine as builder
# Used for running service
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /build
COPY go.mod .
COPY go.sum .
COPY . .
RUN go build -o redis-proxy
WORKDIR /dist
RUN cp /build/redis-proxy .

FROM scratch
COPY --from=builder /dist/redis-proxy /
ENTRYPOINT ["/redis-proxy"]
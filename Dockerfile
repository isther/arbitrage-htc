FROM golang:alpine AS builder

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /build

COPY . .

RUN go mod tidy

RUN go build -o cmd main.go

FROM scratch as runner

COPY --from=builder /build/cmd /
# COPY --from=builder /build/config.yaml /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/cmd"]

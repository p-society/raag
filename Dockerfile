FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags="-s -w" -o tracker ./tracker/cmd/tracker

FROM alpine

RUN apk --no-cache add ca-certificates wget

WORKDIR /app
COPY --from=builder /app/tracker .

EXPOSE 8080 45678

ENV PORT=8080
ENV LIBP2P_PORT=45678
ENV AUTH_SECRET=${AUTH_SECRET:-}
ENV TLS_ENABLED=
ENV TLS_CERT_FILE=
ENV TLS_KEY_FILE=

ENTRYPOINT ["/bin/sh", "-c"]
CMD ["./tracker --http-port $PORT --libp2p-port $LIBP2P_PORT --relay $([ \"$TLS_ENABLED\" = \"true\" ] && echo \"--tls\") $([ -n \"$TLS_CERT_FILE\" ] && echo \"--tls-cert $TLS_CERT_FILE\") $([ -n \"$TLS_KEY_FILE\" ] && echo \"--tls-key $TLS_KEY_FILE\")"]

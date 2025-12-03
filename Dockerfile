ARG GOLANG_VERSION="1.25.5"

FROM golang:${GOLANG_VERSION}-alpine AS builder
ARG LDFLAGS=""
WORKDIR /go/src/github.com/z0rr0/ggp
COPY . .
RUN echo "LDFLAGS = $LDFLAGS"
RUN GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o ./ggp

FROM alpine:3.22
RUN apk --no-cache add ca-certificates
LABEL org.opencontainers.image.authors="me@axv.email" \
    org.opencontainers.image.url="https://hub.docker.com/r/z0rr0/ggp" \
    org.opencontainers.image.documentation="https://github.com/z0rr0/ggp" \
    org.opencontainers.image.source="https://github.com/z0rr0/ggp" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.title="GGP" \
    org.opencontainers.image.description="Golden Gym Predictor Telegram Bot"

COPY --from=builder /go/src/github.com/z0rr0/ggp/ggp /bin/ggp
RUN chmod 0755 /bin/ggp

VOLUME ["/data/"]
ENTRYPOINT ["/bin/ggp"]
CMD ["-config", "/data/config.toml"]

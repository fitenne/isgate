FROM golang:alpine AS builder
RUN --mount=type=cache,id=apk,target=/var/cache/apk \
    apk add gcc musl-dev
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,id=gomod,target=/root/.cache/go-mod \
    go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=1 go build -o isgate

FROM alpine:latest
WORKDIR /app

COPY --from=builder /src/isgate /app/isgate

EXPOSE 8080
ENTRYPOINT ["./isgate"]
CMD ["run"]
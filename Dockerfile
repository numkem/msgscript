FROM golang:1.22-alpine as builder
MAINTAINER Sebastien Bariteau <numkem@numkem.org>

WORKDIR /usr/src/app
COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .
RUN mkdir /dist && \
    go build -o /dist/cli ./cmd/cli/ && \
    go build -o /dist/server ./cmd/server

RUN for i in $(find plugins/ -maxdepth 1 -type d | sed '1d'); do \
      echo "Building plugin $i\n" \
      go build -buildmode=plugin -o /dist/ $i; \
    done


FROM alpine:latest

RUN apk add --upgrade ca-certificates

COPY --from=builder /dist /app
COPY ./docker/entrypoint.sh /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]

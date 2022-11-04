FROM golang:1.19-alpine

ARG TOKEN
ARG ALLOWED_CHAT_ID
ARG DEBUG

RUN apk add ffmpeg

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./

RUN go build -o bot

CMD ["./bot"]
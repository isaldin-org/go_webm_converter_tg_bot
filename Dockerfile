FROM --platform=linux/amd64 golang:1.19-alpine AS build

ARG TOKEN
ARG ALLOWED_CHAT_ID
ARG DEBUG

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 go build -o bot

FROM alpine:3.9.6

RUN apk add ffmpeg

# will be deleted later
RUN mkdir boltdb_files
RUN touch boltdb_files/webms_checksums.db

COPY --from=build /app/bot /app/bot

CMD ["/app/bot"]
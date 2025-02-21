FROM golang:1.23 AS builder
WORKDIR /DispatcherApp

RUN apt-get update && apt-get install -y librdkafka-dev

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o app ./cmd/Dispatcher/

FROM ubuntu:22.04
WORKDIR /DispatcherApp

COPY --from=builder /DispatcherApp/app .
COPY ./config/local.yaml /DispatcherApp/config.yaml

EXPOSE 8081

CMD ["./app"]

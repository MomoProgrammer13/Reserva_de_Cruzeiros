FROM golang:1.19-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o msprocessador ./cmd/msprocessador

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/msprocessador .

CMD ["./msprocessador"]
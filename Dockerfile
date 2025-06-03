FROM --platform=linux/amd64 golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o main ./cmd

FROM --platform=linux/amd64 alpine:latest

WORKDIR /app

COPY --from=builder /app/main .

EXPOSE 6000

CMD ["./main"]

FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o merchant-store .
FROM alpine:3.18
WORKDIR /app
COPY --from=builder /app/merchant-store .
EXPOSE 8090
CMD ["./merchant-store"]
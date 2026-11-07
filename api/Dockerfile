FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/fractal-api ./cmd/fractal-api

FROM alpine:latest
COPY --from=builder /bin/fractal-api /bin/fractal-api
EXPOSE 8080
ENTRYPOINT ["/bin/fractal-api"]

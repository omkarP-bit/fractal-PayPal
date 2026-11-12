# syntax=docker/dockerfile:1

# The build context is the repository root (not ./api) because this image
# bakes in the OPA policies and binary. Render and docker compose both build
# it with the root context.

FROM golang:1.27-alpine AS api-builder
WORKDIR /src/api
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fractal-api ./cmd/fractal-api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/setpassword ./cmd/setpassword

# Copy a static OPA binary. Embedding the policy engine means the API needs a
# single service on Render instead of a separate paid private service per
# environment.
FROM openpolicyagent/opa:1.22.3 AS opa

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=api-builder /out/fractal-api /bin/fractal-api
COPY --from=api-builder /out/migrate /bin/migrate
COPY --from=api-builder /out/setpassword /bin/setpassword
COPY --from=opa /opa /opa
COPY policies/ /policies/
COPY api/scripts/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

EXPOSE 8080
ENTRYPOINT ["/app/entrypoint.sh"]

# ---- build stage ----
FROM golang:1.24-alpine AS builder
ARG APP_DIR=master
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/app ./cmd/${APP_DIR}

# ---- runtime stage ----
FROM alpine:3.20
WORKDIR /app
COPY --from=builder /out/app /app/app
EXPOSE 8080 8081 6060 6061
ENTRYPOINT ["/app/app"]


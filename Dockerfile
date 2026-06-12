# ---- build stage ----
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/pipeline ./cmd

# ---- runtime stage ----
FROM alpine:3.20
WORKDIR /app
COPY --from=builder /out/pipeline /app/pipeline
EXPOSE 6060
ENTRYPOINT ["/app/pipeline"]
CMD ["-workers=8", "-synthetic=1000000"]

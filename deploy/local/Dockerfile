# Forge server Docker image — local/dev build.
# Uses the host Go module cache if available, builds with CGO for SQLite.

FROM golang:1.21-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /forged ./forge/cmd/forged

# ---- runtime ----
FROM alpine:3.19
RUN apk add --no-cache git sqlite-libs ca-certificates
COPY --from=builder /forged /usr/local/bin/forged

RUN adduser -D -h /var/lib/forge forge
USER forge

EXPOSE 8420
ENTRYPOINT ["forged"]
CMD ["--addr", ":8420", "--data", "/var/lib/forge"]

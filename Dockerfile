# Build the static binary.
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache dependencies separately from the source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mydal ./cmd/server

# Distroless: no shell, no package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/mydal /mydal
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/mydal"]

# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api     ./cmd/api  && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cron    ./cmd/cron

# distroless ไม่มี shell/package manager -> image ~15MB และ attack surface แทบเป็นศูนย์
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api     /app/api
COPY --from=build /out/migrate /app/migrate
COPY --from=build /out/cron    /app/cron
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]

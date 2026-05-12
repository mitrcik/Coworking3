ARG REGISTRY=mirror.gcr.io
ARG REGISTRY_LIBRARY_PREFIX=library/

FROM ${REGISTRY}/${REGISTRY_LIBRARY_PREFIX}golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

ARG REGISTRY
ARG REGISTRY_LIBRARY_PREFIX
FROM ${REGISTRY}/${REGISTRY_LIBRARY_PREFIX}alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/server /app/server
COPY web /app/web
EXPOSE 8080
ENV TZ=Europe/Moscow
CMD ["/app/server"]

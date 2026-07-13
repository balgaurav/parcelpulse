FROM golang:1.22-alpine AS build
ARG SERVICE
WORKDIR /src/services/${SERVICE}
COPY services/${SERVICE}/go.mod ./
RUN go mod download
COPY services/${SERVICE}/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D app
COPY --from=build /out/service /service
EXPOSE 8080
USER app
HEALTHCHECK --interval=15s --timeout=3s --retries=3 CMD wget -q -O /dev/null http://localhost:${PORT:-8080}/health || exit 1
ENTRYPOINT ["/service"]

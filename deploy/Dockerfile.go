FROM golang:1.22-alpine AS build
ARG SERVICE
WORKDIR /src/services/${SERVICE}
COPY services/${SERVICE}/go.mod ./
RUN go mod download
COPY services/${SERVICE}/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/service"]

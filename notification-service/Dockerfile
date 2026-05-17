FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd ./cmd
RUN go build -o /out/notification-service ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/notification-service /app/notification-service
CMD ["/app/notification-service"]

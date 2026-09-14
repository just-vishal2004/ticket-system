FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ticket-system .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && \
    adduser -D -u 10001 appuser
COPY --from=build /ticket-system /ticket-system

USER appuser
EXPOSE 8080
ENTRYPOINT ["/ticket-system"]

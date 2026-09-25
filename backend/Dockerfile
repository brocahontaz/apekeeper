FROM golang:1.24-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /apekeeper ./cmd/apekeeper

FROM alpine:3.21

RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 appuser
USER appuser
WORKDIR /app

COPY --from=build /apekeeper /app/apekeeper

EXPOSE 8080

CMD ["./apekeeper"]

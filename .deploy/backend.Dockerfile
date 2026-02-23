FROM golang:1.26.0-alpine AS builder
ENV CGO_ENABLED=0
ENV GOOS=linux
RUN apk update --no-cache && apk add --no-cache tzdata git curl
WORKDIR /build
ADD go.mod .
ADD go.sum .
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o /app/main main/main.go

FROM alpine:3.22 AS temp_backend
WORKDIR /app
COPY --from=builder /app/main /app/main
EXPOSE 8080
CMD ["./main"]
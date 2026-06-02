FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /hodhod ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget && adduser -D -H hodhod
USER hodhod
WORKDIR /app
COPY --from=build /hodhod /app/hodhod
COPY web/miniapp /app/web/miniapp
EXPOSE 8080
ENTRYPOINT ["/app/hodhod"]

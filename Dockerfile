FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /hodhod ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /hodhod /app/hodhod
COPY web/miniapp /app/web/miniapp
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/hodhod"]

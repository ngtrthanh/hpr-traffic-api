FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY static/ static/
ARG VERSION=dev
RUN go build -ldflags "-X main.Version=${VERSION}" -o /app

FROM alpine:3.19
WORKDIR /app
COPY --from=build /app .
COPY data/ data/
CMD ["./app"]

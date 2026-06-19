FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod *.go ./
COPY static/ static/
RUN go build -o /app

FROM alpine:3.19
WORKDIR /app
COPY --from=build /app .
COPY *.csv .
COPY shipping_lanes.geojson .
COPY marnet.geojson .
CMD ["./app"]

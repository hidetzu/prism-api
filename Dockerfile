FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/prism-api \
    ./cmd/prism-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/prism-api /prism-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/prism-api"]

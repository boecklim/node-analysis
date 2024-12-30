FROM golang:1.23.4-alpine3.21 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

COPY cmd/ cmd/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /broadcaster ./cmd/broadcaster/main.go

# Deploy the application binary into a lean image
FROM scratch

WORKDIR /service

COPY --from=build-stage /broadcaster /service/broadcaster

CMD ["/service/broadcaster"]

FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /out/avito-kitchen ./cmd/api

FROM alpine:3.22

RUN addgroup -S app && adduser -S -G app app

COPY --from=build /out/avito-kitchen /usr/local/bin/avito-kitchen

USER app
EXPOSE 8080

CMD ["/usr/local/bin/avito-kitchen"]

FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN go build -o /out/demo-cafe ./cmd/api

FROM alpine:3.22

RUN addgroup -S app && adduser -S -G app app

COPY --from=build /out/demo-cafe /usr/local/bin/demo-cafe

USER app
EXPOSE 8081

CMD ["/usr/local/bin/demo-cafe"]

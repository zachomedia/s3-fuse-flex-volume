# Build goofys driver
FROM golang:1.14-alpine AS goofys

COPY drivers/goofys/main.go /go
RUN go build /go/main.go


# Build deployment container
FROM bash:5

COPY deploy.sh /usr/local/bin
COPY --from=goofys /go/main /goofys-flex-volume

CMD /usr/local/bin/deploy.sh

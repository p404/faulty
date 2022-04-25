FROM golang:alpine as build
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o faulty .

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=build /app/faulty /usr/local/bin/faulty
EXPOSE 8080

ENTRYPOINT [ "faulty" ]

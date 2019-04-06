FROM golang:latest as builder

ENV GO111MODULE=on

WORKDIR /go/src/github.com/appsbyram/seafoodtruck-slack 

COPY . .

RUN go mod tidy

RUN test -z "$(gofmt -l $(find . -type f -name '*.go' -not -path "./vendor/*"))" || { echo "Run \"gofmt -s -w\" on your Golang code"; exit 1; }

RUN go test $(go list ./...) -cover \
    && CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} go build -a -installsuffix cgo -o bot

FROM alpine:latest

COPY entrypoint.sh /

RUN apk --no-cache add ca-certificates 

COPY --from=builder /go/src/github.com/appsbyram/seafoodtruck-slack/bot ./bot 

EXPOSE 80

ENTRYPOINT [ "/entrypoint.sh" ]

CMD [ "start" ]
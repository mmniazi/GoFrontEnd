FROM golang:latest
RUN go get github.com/gorilla/websocket
RUN go get google.golang.org/grpc
RUN go get github.com/go-redis/redis
ADD ./ /go/src/frontend/
CMD cd /go/src/frontend/ && go run src/main.go

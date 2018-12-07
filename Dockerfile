FROM golang:1.9

RUN go get -d github.com/Zauberstuhl/KindergartenBot-golang
WORKDIR $GOPATH/src/github.com/Zauberstuhl/KindergartenBot-golang
RUN go get ./... && go build .
RUN mv KindergartenBot-golang /usr/local/bin/KindergartenBot-golang

RUN adduser -q --disabled-password kgb

USER kgb
WORKDIR /home/kgb
VOLUME /home/kgb

ENTRYPOINT ["/usr/local/bin/KindergartenBot-golang"]

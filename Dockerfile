FROM golang:1.10.2-stretch
ADD . /go/src/github.com/activeeos/nodeos-monitor/
RUN go install github.com/activeeos/nodeos-monitor/cmd/nodeos-monitor

FROM eosio/eos:20180517
COPY --from=0 /go/bin/nodeos-monitor /opt/eosio/bin/.
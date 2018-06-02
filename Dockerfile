FROM golang:1.10.2-stretch
ADD . /go/src/github.com/activeeos/nodeos-monitor/
RUN go install github.com/activeeos/nodeos-monitor/cmd/nodeos-monitor

FROM eosio/eos:v1.0.0
COPY --from=0 /go/bin/nodeos-monitor /opt/eosio/bin/.
RUN apt update -y && apt install -y curl
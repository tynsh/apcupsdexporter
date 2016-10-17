# apcupsdexporter

Metrics exporter for apcupsd network information servers (NIS) for the
[Prometheus](www.prometheus.io)-monitoring-system.

It queries the NIS on each scrape and provides metrics in the prometheus
metrics format.

# WIP
This exporter is a work in progress. It will probably contain a some bugs
and export incorrect metric values. It is advised to wait some time for this
project to stableize before using it in production environments.

# Exported metrics

metric names are made from the following form:

    apcupsd_ups_<metricname>

where `<metricname>` is the lowercase version of the apcupsd metric as seen in the output of apcaccess.

### `apcupsd_ups_status`

The `apcupsd_ups_status` metric is a special case, since it may have multiple
values at the same time. It is represented as the sum of powers of 2 with
exponents from the following list:

* "ONLINE": 0
* "ONBATT": 1
* "CAL": 2
* "TRIM": 3
* "BOOST": 4
* "OVERLOAD": 5
* "LOWBATT": 6
* "REPLACEBATT": 7
* "NOBATT": 8
* "SLAVE": 9
* "SLAVEDOWN": 10
* "SHUTTING DOWN": 11

if a communication error occures, the value is set to zero.

###`apcupsd_ups_alarmdel`

The ALARMDEL metric is thanslated from the NIS response in the following way:

* 30 Seconds: 1
* Low Battery: 2
* No allarm: 3
* 5 Seconds: 4
* Always: 5

### Example
    STATUS:ONBATT LOWBATT
    apcupsd_ups_status 66

Because: `2^1 + 2^6 = 2 + 64 = 66`

# Building and running

    export GOPATH=~/gocode
    go get -u github.com/tynsh/apcupsdexporter
    $GOPATH/bin/apcupsdexporter

# Commandline arguments

You can specify the address of the NIS and the http endpoint using the commandline interface:

    -nis.address string
          colon separated host and port of the Network information Server to monitor (default "localhost:3551")
    -nis.timeout int
          timeout lengh for requests to the Network information Server (default 30)
    -web.endpoint string
          HTTP Endpoint for metrics (default "/metrics")
    -web.listenaddress string
          colon separated host and port to listen on for metric requests (default ":9191")

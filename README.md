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
values at the same time. It uses the label "type" to describe which type of
state the ups reports. A value of 1 represents the state as being reported, 0
means it is not. The following are all possible states:

* ONLINE
* ONBATT
* CAL
* COMMLOST
* TRIM
* BOOST
* OVERLOAD
* LOWBATT
* REPLACEBATT
* NOBATT
* SLAVE
* SLAVEDOWN
* SHUTTING DOWN

If the exporter itself is not able to contact the NIS the COMMLOST type is set
to 1.

#### Example
    STATUS:ONBATT LOWBATT
    apcupsd_ups_status{type="BOOST"} 0
    apcupsd_ups_status{type="LOWBATT"} 1
    apcupsd_ups_status{type="ONBATT"} 1
    apcupsd_ups_status{type="CAL"} 0
    apcupsd_ups_status{type="OVERLOAD"} 0
    …

###`apcupsd_ups_alarmdel`

The ALARMDEL metric is thanslated from the NIS response in the following way:

* 30 Seconds: 1
* Low Battery: 2
* No allarm: 3
* 5 Seconds: 4
* Always: 5

###`apcupsd_ups_sense`

* Auto Adjust: 1
* Low: 2
* Medium: 3
* High: 4

###`apcupsd_ups_selftest`

* NO: 1
* NG: 2
* WN: 3
* IP: 4
* OK: 5
* BT: 6

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
          colon separated host and port to listen on for metric requests (default ":9385")

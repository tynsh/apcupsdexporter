//   Copyright 2016 Tobias Wackenhut
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package main

import (
	"encoding/binary"
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	prommodel "github.com/prometheus/client_model/go"
	log "github.com/prometheus/common/log"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// config needs to be stored globaly because we cannot give arguments to the
// prometheus gatherer function
var config config_t

// Queries the NIS and returns the raw response as string
//
// We need to send the following request values:
// - int16 specifying the length of the request message
// - []byte slice with the actual request message
// the status command is "status"
func nis_request() string {

	// the apcupsd NIS expects an int16 bevore the status command
	var servercommand []byte = []byte("status")
	var strlength int16 = int16(len(servercommand))

	var addr string = *(config.NISAddress)
	var timeout time.Duration = time.Duration(*(config.NISRequestTimeout)) * time.Second

	// connect to nis
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		log.Error("could not connect to apcupsd server")
		log.Error(err.Error())
		return ""
	}
	defer conn.Close()

	// encode the messages as BigEndian and send them
	binary.Write(conn, binary.BigEndian, strlength)     // size of the following command
	binary.Write(conn, binary.BigEndian, servercommand) // command as ascii []byte

	// receive []byte array from Server
	ServerResponse := make([]byte, 0, 4096) // do not expect more than 4KB of response Data
	var length_response int16               // int16 as byte array

	for {
		// first read the number of bytes that will follow
		err := binary.Read(conn, binary.BigEndian, &length_response)
		if err != nil {
			if err != io.EOF {
				log.Error("error when receiving int16 with size of following data")
			}
		}

		// check if there is more data to read, or if the server just didn't return an EOF
		if length_response == 0 {
			break
		}

		// receive data from NIS
		recv_buffer := make([]byte, length_response) // create buffer of the from nis reported message size
		n, err := conn.Read(recv_buffer)
		if err != nil {
			if err != io.EOF {
				log.Warn("read error:", err)
			}
			break
		}
		log.Debug("got ", n, " bytes.")
		ServerResponse = append(ServerResponse, recv_buffer[:n]...)

	}
	log.Debug("total size:", len(ServerResponse))

	// convert []byte to string
	ServerResponseString := string(ServerResponse)
	log.Debug("Raw nis response:")
	log.Debug(ServerResponseString)

	return ServerResponseString
}

// configuration struct
type config_t struct {
	NISAddress        *string
	webListenAddress  *string
	NISRequestTimeout *int
	HTTPEndpoint      *string
}

// Initialize configuration variables with flag
func read_commandline() config_t {

	var config config_t

	config.NISAddress = flag.String("nis.address", "localhost:3551", "colon separated host and port of the Network information Server to monitor")
	config.webListenAddress = flag.String("web.listenaddress", ":9191", "colon separated host and port to listen on for metric requests")
	config.HTTPEndpoint = flag.String("web.endpoint", "/metrics", "HTTP Endpoint for metrics")
	config.NISRequestTimeout = flag.Int("nis.timeout", 30, "timeout lengh for requests to the Network information Server")

	flag.Parse()

	return config

}

// function to convert status value from NIS response to numeric metric
// number returned is the sum of 2^statuscode where statuscode is a numeric
// representation of a value reported from NIS. This conversion is neccessary
// because the status reported from NIS may have multiple space separated values
func convert_status(value string) float64 {
	status_codes := map[string]float64{
		"ONLINE":        0,
		"ONBATT":        1,
		"CAL":           2,
		"TRIM":          3,
		"BOOST":         4,
		"OVERLOAD":      5,
		"LOWBATT":       6,
		"REPLACEBATT":   7,
		"NOBATT":        8,
		"SLAVE":         9,
		"SLAVEDOWN":     10,
		"SHUTTING DOWN": 11,
		// COMMLOST not included, since it is an error
	}

	// trim spaces and split NIS reported values
	value = strings.TrimSpace(value)
	statuses := strings.Split(value, " ")

	// sum all given values as powers of 2
	var statuscode float64 = 0
	for _, statusstring := range statuses {
		statusstring = strings.TrimSpace(statusstring)
		if status_code_to_add, noError := status_codes[statusstring]; noError {
			statuscode += math.Pow(2, status_code_to_add)
		}
	}

	// if statuscode is 0, this means we either have no data (e.g. "N/A" value) or something else is wrong
	return statuscode
}

// TODO: alarmdel
// sense
// selftest (?)

func convert_alarmdel(value string) float64 {
	status_codes := map[string]float64{
		"30 Seconds":  1,
		"Low Battery": 2,
		"No alarm":    3,
		"5 Seconds":   4,
		"Always":      5,
	}

	return status_codes[value]
}

// converts a string value from nis to an int64
// expects a string without trailing whitespaces
func convert_float64(value string) float64 {
	// first remove units
	units := []string{
		" Minutes",
		" Seconds",
		" Percent",
		" Volts",
		" Watts",
		" Hz",
		" C"}
	for _, unit := range units {
		value = strings.TrimRight(value, unit)
	}

	// now we simply try to convert the value to an integer and return 0 if it fails
	// seems more robust than comparing each possible non-integer value to a specific error-string
	numeric_value, err := strconv.ParseFloat(value, 64)
	if err != nil {
		numeric_value = 0
	}
	return numeric_value
}

// struct containing initialized collectors
type metrics_t struct {
	status    prometheus.Gauge
	linev     prometheus.Gauge
	loadpct   prometheus.Gauge
	bcharge   prometheus.Gauge
	timeleft  prometheus.Gauge
	mbattchg  prometheus.Gauge
	mintimel  prometheus.Gauge
	maxtime   prometheus.Gauge
	maxlinev  prometheus.Gauge
	minlinev  prometheus.Gauge
	outputv   prometheus.Gauge
	sense     prometheus.Gauge
	dwake     prometheus.Gauge
	dshutd    prometheus.Gauge
	dlowbatt  prometheus.Gauge
	lotrans   prometheus.Gauge
	hitrans   prometheus.Gauge
	retpct    prometheus.Gauge
	itemp     prometheus.Gauge
	alarmdel  prometheus.Gauge
	battv     prometheus.Gauge
	linefreq  prometheus.Gauge
	tonbatt   prometheus.Gauge
	nomoutv   prometheus.Gauge
	nombattv  prometheus.Gauge
	extbatts  prometheus.Gauge
	badbatts  prometheus.Gauge
	cumonbatt prometheus.Gauge
	humidity  prometheus.Gauge
	ambtemp   prometheus.Gauge
}

// collector struct must be global to be readable between scrapes
var metrics metrics_t

// request information from NIS and update metrics
func update_metrics() {
	raw_response := nis_request()

	// emulate COMMLOST if we couldn't reach NIS
	if len(raw_response) == 0 {
		raw_response = "STATUS:COMMLOST"
	}

	// loop over each line of the nis response
	for _, line := range strings.Split(raw_response, "\n") {
		log.Debug(line)

		// split line by colons, since NIS returns metrics in the form key:value
		line = strings.TrimSpace(line)
		split_line := strings.Split(line, ":")

		if len(split_line) == 2 { // must have exactly one colon, else not a metric
			entry := strings.TrimSpace(split_line[0]) // trim spaces of entry and value
			value := strings.TrimSpace(split_line[1])

			// switch over the possible entrys and create metrics on demand
			switch entry {
			case "STATUS":
				if metrics.status == nil {
					metrics.status = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_status",
						Help: "UPS Status. Value 0=error 1=ONLINE, for the rest see documentation",
					})
					prometheus.MustRegister(metrics.status)
				}
				metrics.status.Set(convert_status(value))
			case "LINEV":
				if metrics.linev == nil {
					metrics.linev = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_linev",
						Help: "Current input line voltage",
					})
					prometheus.MustRegister(metrics.linev)
				}
				metrics.linev.Set(convert_float64(value))
			case "LOADPCT":
				if metrics.loadpct == nil {
					metrics.loadpct = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_loadpct",
						Help: "Percentage of UPS load capacity used as estimated by UPS",
					})
					prometheus.MustRegister(metrics.loadpct)
				}
				metrics.loadpct.Set(convert_float64(value))
			case "BCHARGE":
				if metrics.bcharge == nil {
					metrics.bcharge = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_bcharge",
						Help: "Current battery capacity charge percentage",
					})
					prometheus.MustRegister(metrics.bcharge)
				}
				metrics.bcharge.Set(convert_float64(value))
			case "TIMELEFT":
				if metrics.timeleft == nil {
					metrics.timeleft = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_timeleft",
						Help: "Remaining runtime left on battery as estimated by the UPS",
					})
					prometheus.MustRegister(metrics.timeleft)
				}
				metrics.timeleft.Set(convert_float64(value))
			case "MBATTCHG":
				if metrics.mbattchg == nil {
					metrics.mbattchg = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_mbattchg",
						Help: "Min battery charge % (BCHARGE) required for system shutdown",
					})
					prometheus.MustRegister(metrics.mbattchg)
				}
				metrics.mbattchg.Set(convert_float64(value))
			case "MINTIMEL":
				if metrics.mintimel == nil {
					metrics.mintimel = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_mintimel",
						Help: "Min battery runtime (MINUTES) required for system shutdown",
					})
					prometheus.MustRegister(metrics.mintimel)
				}
				metrics.mintimel.Set(convert_float64(value))
			case "MAXTIME":
				if metrics.maxtime == nil {
					metrics.maxtime = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_maxtime",
						Help: "Max battery runtime (TIMEOUT) after which system is shutdown",
					})
					prometheus.MustRegister(metrics.maxtime)
				}
				metrics.maxtime.Set(convert_float64(value))
			case "MAXLINEV":
				if metrics.maxlinev == nil {
					metrics.maxlinev = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_maxlinev",
						Help: "Maximum input line voltage since apcupsd started",
					})
					prometheus.MustRegister(metrics.maxlinev)
				}
				metrics.maxlinev.Set(convert_float64(value))
			case "MINLINEV":
				if metrics.minlinev == nil {
					metrics.minlinev = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_minlinev",
						Help: "Min (observed) input line voltage since apcupsd started",
					})
					prometheus.MustRegister(metrics.minlinev)
				}
				metrics.minlinev.Set(convert_float64(value))
			case "OUTPUTV":
				if metrics.outputv == nil {
					metrics.outputv = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_outputv",
						Help: "Current UPS output voltage",
					})
					prometheus.MustRegister(metrics.outputv)
				}
				metrics.outputv.Set(convert_float64(value))
			case "SENSE":
				if metrics.sense == nil {
					metrics.sense = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_sense",
						Help: "Current UPS sensitivity setting for voltage fluctuations",
					})
					prometheus.MustRegister(metrics.sense)
				}
				metrics.sense.Set(convert_float64(value))
			case "DWAKE":
				if metrics.dwake == nil {
					metrics.dwake = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_dwake",
						Help: "Time UPS waits after power off when the power is restored",
					})
					prometheus.MustRegister(metrics.dwake)
				}
				metrics.dwake.Set(convert_float64(value))
			case "DSHUTD":
				if metrics.dshutd == nil {
					metrics.dshutd = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_dshutd",
						Help: "Delay before UPS powers down after command received",
					})
					prometheus.MustRegister(metrics.dshutd)
				}
				metrics.dshutd.Set(convert_float64(value))
			case "DLOWBATT":
				if metrics.dlowbatt == nil {
					metrics.dlowbatt = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_dlowbatt",
						Help: "Low battery signal sent when this much runtime remains",
					})
					prometheus.MustRegister(metrics.dlowbatt)
				}
				metrics.dlowbatt.Set(convert_float64(value))
			case "LOTRANS":
				if metrics.lotrans == nil {
					metrics.lotrans = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_lotrans",
						Help: "Input line voltage below which UPS will switch to battery",
					})
					prometheus.MustRegister(metrics.lotrans)
				}
				metrics.lotrans.Set(convert_float64(value))
			case "HITRANS":
				if metrics.hitrans == nil {
					metrics.hitrans = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_hitrans",
						Help: "Input line voltage above which UPS will switch to battery",
					})
					prometheus.MustRegister(metrics.hitrans)
				}
				metrics.hitrans.Set(convert_float64(value))
			case "RETPCT":
				if metrics.retpct == nil {
					metrics.retpct = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_retpct",
						Help: "Battery charge % required after power off to restore power",
					})
					prometheus.MustRegister(metrics.retpct)
				}
				metrics.retpct.Set(convert_float64(value))
			case "ITEMP":
				if metrics.itemp == nil {
					metrics.itemp = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_itemp",
						Help: "UPS internal temperature in degrees Celcius",
					})
					prometheus.MustRegister(metrics.itemp)
				}
				metrics.itemp.Set(convert_float64(value))
			case "ALARMDEL":
				if metrics.alarmdel == nil {
					metrics.alarmdel = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_alarmdel",
						Help: "Delay period before UPS starts sounding alarm",
					})
					prometheus.MustRegister(metrics.alarmdel)
				}
				metrics.alarmdel.Set(convert_alarmdel(value))
			case "BATTV":
				if metrics.battv == nil {
					metrics.battv = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_battv",
						Help: "Current battery voltage",
					})
					prometheus.MustRegister(metrics.battv)
				}
				metrics.battv.Set(convert_float64(value))
			case "LINEFREQ":
				if metrics.linefreq == nil {
					metrics.linefreq = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_linefreq",
						Help: "Current line frequency in Hertz",
					})
					prometheus.MustRegister(metrics.linefreq)
				}
				metrics.linefreq.Set(convert_float64(value))
			case "TONBATT":
				if metrics.tonbatt == nil {
					metrics.tonbatt = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_tonbatt",
						Help: "Seconds currently on battery",
					})
					prometheus.MustRegister(metrics.tonbatt)
				}
				metrics.tonbatt.Set(convert_float64(value))
			case "NOMOUTV":
				if metrics.nomoutv == nil {
					metrics.nomoutv = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_nomoutv",
						Help: "Nominal output voltage to supply when on battery power",
					})
					prometheus.MustRegister(metrics.nomoutv)
				}
				metrics.nomoutv.Set(convert_float64(value))
			case "NOMBATTV":
				if metrics.nombattv == nil {
					metrics.nombattv = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_nombattv",
						Help: "Nominal battery voltage",
					})
					prometheus.MustRegister(metrics.nombattv)
				}
				metrics.nombattv.Set(convert_float64(value))
			case "EXTBATTS":
				if metrics.extbatts == nil {
					metrics.extbatts = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_extbatts",
						Help: "Number of external batteries (for XL models)",
					})
					prometheus.MustRegister(metrics.extbatts)
				}
				metrics.extbatts.Set(convert_float64(value))
			case "BADBATTS":
				if metrics.badbatts == nil {
					metrics.badbatts = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_badbatts",
						Help: "Number of bad external battery packs (for XL models)",
					})
					prometheus.MustRegister(metrics.badbatts)
				}
				metrics.badbatts.Set(convert_float64(value))
			case "CUMONBATT":
				if metrics.cumonbatt == nil {
					metrics.cumonbatt = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_cumonbatt",
						Help: "Cumulative seconds on battery since apcupsd startup",
					})
					prometheus.MustRegister(metrics.cumonbatt)
				}
				metrics.cumonbatt.Set(convert_float64(value))
			case "HUMIDITY":
				if metrics.humidity == nil {
					metrics.humidity = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_humidity",
						Help: "Ambient humidity",
					})
					prometheus.MustRegister(metrics.humidity)
				}
				metrics.humidity.Set(convert_float64(value))
			case "AMBTEMP":
				if metrics.ambtemp == nil {
					metrics.ambtemp = prometheus.NewGauge(prometheus.GaugeOpts{
						Name: "apcupsd_ups_ambtemp",
						Help: "Ambient temperature",
					})
					prometheus.MustRegister(metrics.ambtemp)
				}
				metrics.ambtemp.Set(convert_float64(value))
			default:
				// do nothing because the entry is not an interesting metric
			}
		}
	}
}

// wrapper around the default gatherer, because we want to update the metrics
// before the Gather function of the default gatherer is called.
func gatherer_wrapper() ([]*prommodel.MetricFamily, error) {
	update_metrics()
	return prometheus.DefaultGatherer.Gather()
}

func main() {
	config = read_commandline()

	// initialize http handler
	var gatherer prometheus.GathererFunc = gatherer_wrapper
	var handlerOpts promhttp.HandlerOpts
	http.Handle(*(config.HTTPEndpoint), promhttp.HandlerFor(gatherer, handlerOpts))

	// start webserver
	log.Fatal(http.ListenAndServe(*(config.webListenAddress), nil))
}

package consumer

import (
	"github.com/Intelicolab/icinga2otel/internal/config"
	"github.com/Intelicolab/icinga2otel/internal/objectcache"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	apiLog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/attribute"
	"time"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	semconv "go.opentelemetry.io/otel/semconv/v1.32.0"
)

var (
	//hold regex in global to avoid frequent recompilation
	re_float *regexp.Regexp = regexp.MustCompile(`[-+]?\d*\.?\d+`)
)


// Icinga Event struct
type IcingaEvent struct {
	Acknowledgement bool        `json:"acknowledgement"`
	CheckResult     CheckResult `json:"check_result"`
	DowntimeDepth   int         `json:"downtime_depth"`
	Host            string      `json:"host"`
	Service         string      `json:"service"`
	Timestamp       float64     `json:"timestamp"`
	Type            string      `json:"type"`
	State           int         `json:"state"`
	StateType       int         `json:"state_type"`
	Comment		Comment	    `json:"comment"`
	Downtime	Downtime    `json:"downtime"`
	Text		string	    `json:"text"`
	Users		[]string    `json:"users"`
	Author		string    `json:"author"`
	ObjectName		string    `json:"object_name"`
	ObjectType		string    `json:"object_type"`
}

// Struct for the nested 'check_result'
type CheckResult struct {
	Active      bool   `json:"active"`
	CheckSource string `json:"check_source"`
	Command        Command `json:"command"`
	ExecutionEnd   float64     `json:"execution_end"`
	ExecutionStart float64     `json:"execution_start"`
	ExitStatus     int         `json:"exit_status"`
	Output         string      `json:"output"`
	// Specific struct for the structured performance data array.  Has custom handler since this array may come as []string, or []PerfdataValue objects
	PerformanceData   PerfdataValues `json:"performance_data"`
	PreviousHardState int            `json:"previous_hard_state"`
	ScheduleEnd       float64        `json:"schedule_end"`
	ScheduleStart     float64        `json:"schedule_start"`
	SchedulingSource  string         `json:"scheduling_source"`
	State             int            `json:"state"`
	TTL               int            `json:"ttl"`
	Type              string         `json:"type"`
}

// Struct for the structured performance data objects within the array
// Note: Warn, Crit, Min, or Max could be null or a value
type PerfdataValue struct {
	Counter bool        `json:"counter"`
	Crit    *float64    `json:"crit"`
	Label   string      `json:"label"`
	Max     *float64 `json:"max"`
	Min     *float64 `json:"min"`
	Type    string      `json:"type"`
	Unit    string      `json:"unit"`
	Value   float64     `json:"value"`
	Warn    *float64 `json:"warn"`
}

// Struct for structured Comment
type Comment struct {
    Name          string         `json:"__name"`
    Author        string         `json:"author"`
    EntryTime     float64        `json:"entry_time"`
    EntryType     int            `json:"entry_type"`
    ExpireTime    float64        `json:"expire_time"`
    HostName      string         `json:"host_name"`
    LegacyID      int            `json:"legacy_id"`
    CommentName   string         `json:"name"`
    Package       string         `json:"package"`
    Persistent    bool           `json:"persistent"`
    ServiceName   string         `json:"service_name"`
    Sticky        bool           `json:"sticky"`
    Templates     []string       `json:"templates"`
    Text          string         `json:"text"`
    Type          string         `json:"type"`
    Version       float64        `json:"version"`
    Zone          string         `json:"zone"`
}

// Struct for Downtime
type Downtime struct {
    Name              string        `json:"__name"`
    Author            string        `json:"author"`
    AuthoritativeZone string        `json:"authoritative_zone"`
    Comment           string        `json:"comment"`
    ConfigOwner       string        `json:"config_owner"`
    ConfigOwnerHash   string        `json:"config_owner_hash"`
    Duration          int           `json:"duration"`
    EndTime           int64         `json:"end_time"`
    EntryTime         float64       `json:"entry_time"`
    Fixed             bool          `json:"fixed"`
    HostName          string        `json:"host_name"`
    LegacyID          int           `json:"legacy_id"`
    DowntimeName      string        `json:"name"`
    Package           string        `json:"package"`
    Parent            string        `json:"parent"`
    RemoveTime        int           `json:"remove_time"`
    ScheduledBy       string        `json:"scheduled_by"`
    ServiceName       string        `json:"service_name"`
    StartTime         int64         `json:"start_time"`
    Templates         []string      `json:"templates"`
    TriggerTime       float64       `json:"trigger_time"`
    TriggeredBy       string        `json:"triggered_by"`
    Triggers          []string      `json:"triggers"`
    Type              string        `json:"type"`
    Version           float64       `json:"version"`
    Zone              string        `json:"zone"`
}


// Handle Command as string or []string
type Command []string

func (c *Command) UnmarshalJSON(data []byte) error {
	var stringSlice []string
	var stringString string

	if err := json.Unmarshal(data, &stringSlice); err == nil && len(stringSlice) > 0 {
		*c = stringSlice
	} else if err := json.Unmarshal(data, &stringString); err == nil {
		*c = []string{stringString}
	}
	return nil

}

// handle perfdata as []string or slice of PerfdataValue structs
type PerfdataValues []PerfdataValue

func (p *PerfdataValues) UnmarshalJSON(data []byte) error {
	var stringSlice []string
	var objectSlice []PerfdataValue
	var err error
	*p = nil

	if err = json.Unmarshal(data, &stringSlice); err == nil {
		for _, s := range stringSlice {
			metric := PerfdataValue{}
			if err = metric.parseString(s); err == nil {
				*p = append(*p, metric)
			}
		}
	} else if err := json.Unmarshal(data, &objectSlice); err == nil {
		slog.Debug("Parsed object", "object", string(data))
		*p = objectSlice
	} else {
		return fmt.Errorf("could not unmarshal performance_data %w", err)
	}

	return nil
}

//Use to handle a performance data string, as spec'ed in https://www.monitoring-plugins.org/doc/guidelines.html#AEN197
func (pv *PerfdataValue) parseString(data string) error {

	slog.Debug("Parsing perfdata string","perfdata", data)
	dataTokens := strings.Split(data, ";")

	var tokens [5]string
	copy(tokens[:5], dataTokens)

	metricTokens := strings.Split(tokens[0], "=")

	if len(metricTokens) < 2 {
		return fmt.Errorf("Unparseable perfdata string")
	}

	if labelPart := strings.Trim(metricTokens[0], " '"); labelPart == "" {
		return fmt.Errorf("Unparseable perfdata string")
	} else {
		pv.Label = labelPart

	}

	valuePart := strings.Trim(metricTokens[1], " ")
	numericString := re_float.FindString(valuePart)

	pv.Value, _ = strconv.ParseFloat(numericString, 64) //failure to match number will return 0

	pv.Unit = valuePart[strings.Index(valuePart, numericString)+len(numericString):] //pull out substring after found numeric value


	pv.Warn = pv.parseAttrValue(tokens[1])
	pv.Crit = pv.parseAttrValue(tokens[2])
	pv.Min = pv.parseAttrValue(tokens[3])
	pv.Max = pv.parseAttrValue(tokens[4])
	pv.Counter = pv.Unit == "c"

	slog.Debug("Finish parsing perfdata string")
	return nil

}

//Get the attributes to add from the perfdata fields, if they were present
func (pv *PerfdataValue) MetricAttributes() (attrs []attribute.KeyValue) {

	attrs = append(attrs, attribute.String("metric.orig.name", pv.Label))

	if pv.Warn != nil {
		attrs = append(attrs, attribute.Float64("warn", *pv.Warn))
	}
	if pv.Crit != nil {
		attrs = append(attrs, attribute.Float64("crit", *pv.Crit))
	}
	if pv.Min != nil {
		attrs = append(attrs, attribute.Float64("min", *pv.Min))
	}
	if pv.Max != nil {
		attrs = append(attrs, attribute.Float64("max", *pv.Max))
	}

	return

}

func (pv *PerfdataValue) MungeMetricName() (munged string) {

	munged = pv.Label

	if len(config.Config.MetricMunges) != 0 {
		for _, m := range config.Config.MetricMunges {
			munged = m.Search.ReplaceAllString(pv.Label, m.Replace)
			if munged != pv.Label { //first match that results in change "wins"
				slog.Debug("munged metric name", "orig", pv.Label, "munged", munged)
				return
			}
		}
	}

	return

}

// Pass in additional attributes to add (clone the slice!), and the timestamp for the metric.  this will add the PerfdataValue Metric Attibutes
func (pv *PerfdataValue) GetOtelMetric(attrs []attribute.KeyValue, metricTime time.Time) (pdv_metric metricdata.Metrics) {

	var pointAttributes []attribute.KeyValue = append(attrs, pv.MetricAttributes()...)



	if pv.Counter {
		slog.Debug("Processing as counter.")
		pdv_metric = metricdata.Metrics{
			Name: pv.MungeMetricName(),
			Data: metricdata.Sum[int64]{
				IsMonotonic: true,
				Temporality: metricdata.CumulativeTemporality,
				DataPoints: []metricdata.DataPoint[int64]{
					{
						Time:       metricTime,
						Value:      int64(pv.Value),
						Attributes: attribute.NewSet(pointAttributes...),
					},
				},
			},
		}
	} else {
		slog.Debug("Processing as gauge.")
		pdv_metric = metricdata.Metrics{
			Name: pv.MungeMetricName(),
			Data: metricdata.Gauge[float64]{
				DataPoints: []metricdata.DataPoint[float64]{
					{
						Time:       metricTime,
						Value:      pv.Value,
						Attributes: attribute.NewSet(pointAttributes...),
					},
				},
			},
		}
	}
	return
}


/**
Parse out the range if given, and evaluate best numeric value to use from range.
If no range given, parse out the float value
Or return nil if not present or not parseable
**/
func (pv *PerfdataValue) parseAttrValue(threshold string) *float64 {

	if (threshold == "") { return nil }

	var r float64
	var err error

	// Simple case, and likely common, so if this works, don't waste time on anything else.
	if r, err = strconv.ParseFloat(threshold, 64); err != nil {
		return &r
	}

	tokens := strings.Split(threshold, ":")

	if len(tokens) == 1 { //no range, just parse Float from string, if possible
		numericString := re_float.FindString(tokens[0])
		if numericString == "" {
			return nil
		}
		r, _ = strconv.ParseFloat(numericString, 64)
	} else {
		numericStringLow := re_float.FindString(tokens[0])
		numericStringHi := re_float.FindString(tokens[1])

		if numericStringLow == "" && numericStringHi == "" {
			return nil
		} // no numeric values at all in range

		if numericStringLow == "" {
			r, _ = strconv.ParseFloat(numericStringHi, 64)
		} else if numericStringHi == "" {
			r, _ = strconv.ParseFloat(numericStringLow, 64)
		} else {
			// We have two actual numbers for the range.  We will guess, somewhat arbitrarily, that the largest absolute value is
			// the most significant ( think of 0:10, 0:20...we'd probably want 10 and 20)
			rLow, _ := strconv.ParseFloat(numericStringLow, 64)
			rHi, _ := strconv.ParseFloat(numericStringHi, 64)
			if math.Abs(rHi) > math.Abs(rLow) {
				r = rHi
			} else {
				r = rLow
			}
		}
	}

	return &r
}

// ----- Icinga Monitored Object Types and Mthods

type MonObjectType int

const (
	MonHost MonObjectType = iota
	MonService
)

func (o MonObjectType) String() string {
	var ObjectNames = map[MonObjectType]string{
		MonHost: "Host",
		MonService: "Service",
	}
	return ObjectNames[o]
}

// ----- Icinga State Types and Methods

type HostState int
type ServiceState int

// --- Icinga States
const (
    StateUp HostState = iota
    StateDown
    StateUnreachable
)

const (
    StateOk ServiceState = iota
    StateWarn
    StateCrit
    StateUnknown
)

var hostStateName = map[HostState]string{
    StateUp:      "UP",
    StateDown: "DOWN",
    StateUnreachable:     "UNREACHABLE",
}

var serviceStateName = map[ServiceState]string{
    StateOk:      "OK",
    StateWarn: "WARNING",
    StateCrit:     "CRITICAL",
    StateUnknown:     "UNKNOWN",
}

func (s HostState) String() string {
    return hostStateName[s]
}

func (s ServiceState) String() string {
    return serviceStateName[s]
}

var hostSeverityNumber = map[HostState]apiLog.Severity {
	StateUp:      apiLog.SeverityInfo,
	StateDown: apiLog.SeverityError,
	StateUnreachable:     apiLog.SeverityFatal,
}

var serviceSeverityNumber = map[ServiceState]apiLog.Severity {
	StateOk:      apiLog.SeverityInfo,
	StateWarn: apiLog.SeverityWarn,
	StateCrit:     apiLog.SeverityError,
	StateUnknown:     apiLog.SeverityFatal,
}

func (s ServiceState) Severity()  apiLog.Severity {
	return serviceSeverityNumber[s]
}

func (s HostState) Severity()  apiLog.Severity {
	return hostSeverityNumber[s]
}


func (e IcingaEvent) MonObjectType() MonObjectType {

	var r MonObjectType = MonService

	_, check, _ := e.MonObjectNames()
	if check == "" {
		r = MonHost
	}
	return r
}



// Convenience function for retrieving the monitor object names references in the event.
func (e IcingaEvent) MonObjectNames() (hostname string, servicename string, objectname string) {

	hostname = e.Host
	servicename = e.Service
	switch e.Type {
	case "CommentAdded", "CommentRemoved":
		hostname = e.Comment.HostName
		servicename = e.Comment.ServiceName
	case "DowntimeAdded", "DowntimeRemoved", "DowntimeStarted", "DowntimeTriggered":
		hostname = e.Downtime.HostName
		servicename = e.Downtime.ServiceName
	}



	objectname = hostname + "!" + servicename

	return
}

func (e IcingaEvent) StateName() (name string) {

	name = ServiceState(e.State).String()
	if e.MonObjectType() == MonHost {
		name = HostState(e.State).String()
	}
	return

}

func (e IcingaEvent) StateOtelSeverity() (severity apiLog.Severity) {

	severity = ServiceState(e.State).Severity()
	if e.MonObjectType() == MonHost {
		severity = HostState(e.State).Severity()
	}
	return
}

// Convenience function for generating a log message for the event
func (e IcingaEvent) LogMessage() string {

	var msg = "Event Received" //generic

	_, _, objectname := e.MonObjectNames()
	objectType := e.MonObjectType()

	var state string = e.StateName()

	switch e.Type {
	case "StateChange":
		if e.State == 0 {
			msg = fmt.Sprintf("%s %s recovered.", objectType, objectname)
		} else {
			msg = fmt.Sprintf("%s %s is in state %s.", objectType, objectname, state)
		}
	case "CommentAdded":
		msg = fmt.Sprintf("%s added comment to %s %s: %s", e.Comment.Author, objectType, objectname, e.Comment.Text)
	case "CommentRemoved":
		msg = fmt.Sprintf("Comment from %s removed from %s %s: %s", e.Comment.Author, objectType, objectname, e.Comment.Text)
	case "Notification":
		msg = fmt.Sprintf("Notification for %s %s, %s", objectType, objectname, e.Text)
	case "DowntimeAdded":
		msg = fmt.Sprintf("%s added downtime to %s %s: %s", e.Downtime.Author, objectType, objectname, e.Downtime.Comment)
	case "DowntimeRemoved":
		msg = fmt.Sprintf("Downtime from %s removed from %s %s: %s", e.Downtime.Author, objectType, objectname, e.Downtime.Comment)
	case "DowntimeStarted":
		msg = fmt.Sprintf("downtime started for %s %s: %s", objectType, objectname, e.Downtime.Comment)
	case "DowntimeTriggered":
		msg = fmt.Sprintf("downtime triggered for %s %s: %s", objectType, objectname, e.Downtime.Comment)
	case "AcknowledgementSet":
		msg = fmt.Sprintf("State %s acknowledged by %s for %s %s", state, e.Author, objectType, objectname)
	case "AcknowledgementCleared":
		msg = fmt.Sprintf("Acknowledgement removed from %s %s", objectType, objectname)
	}


	if e.CheckResult.Output != "" {
		msg = fmt.Sprintf("%s Output:%s", msg, e.CheckResult.Output)
	}

	return msg
}

func (e IcingaEvent) OtelLogAttributes() (attrs []apiLog.KeyValue) {

	hostname, servicename, _ := e.MonObjectNames()
	attrs = append(attrs, apiLog.KeyValueFromAttribute(semconv.HostName(hostname)))
	attrs = append(attrs, apiLog.KeyValueFromAttribute(semconv.ServiceName(servicename)))
	attrs = append(attrs, apiLog.String("type", e.Type))
	attrs = append(attrs, apiLog.String("object_type", e.MonObjectType().String()))

	attrs = append(attrs, e.ObjectLogAttributes()...)

	switch e.Type {
	case "StateChange":
		attrs = append(attrs, apiLog.String("state",e.StateName()))
	case "CommentAdded", "CommentRemoved":
		attrs = append(attrs, apiLog.String("author",e.Comment.Author))
		attrs = append(attrs, apiLog.String("comment_id",e.Comment.CommentName))
	case "Notification":
		userValues := make([]apiLog.Value, len(e.Users))
		for i, val := range e.Users {
			userValues[i] = apiLog.StringValue(val)
		}
		attrs = append(attrs, apiLog.String("state",e.StateName()))
		attrs = append(attrs, apiLog.Slice("users",userValues...))
	case "DowntimeAdded", "DowntimeRemoved", "DowntimeStarted", "DowntimeTriggered":
		attrs = append(attrs, apiLog.String("author",e.Downtime.Author))
		attrs = append(attrs, apiLog.String("downtime_id",e.Downtime.DowntimeName))
		attrs = append(attrs, apiLog.Int64("start_time",e.Downtime.StartTime))
		attrs = append(attrs, apiLog.Int64("end_time",e.Downtime.EndTime))
		attrs = append(attrs, apiLog.Int64("trigger_time", int64(e.Downtime.EndTime)))

	case "AcknowledgementSet":
		attrs = append(attrs, apiLog.String("state",e.StateName()))
		attrs = append(attrs, apiLog.String("author",e.Author))
	case "AcknowledgementCleared":
		attrs = append(attrs, apiLog.String("state",e.StateName()))
	}

	return

}

func (e IcingaEvent) ObjectLogAttributes() (attrs []apiLog.KeyValue) {

	if len(config.Config.LogAttrs) == 0 {
		return
	}

	hostname, servicename, _ := e.MonObjectNames()

	host := objectcache.GetHost(hostname)
	service := objectcache.GetService(hostname, servicename)
	objType := e.MonObjectType()

	for _, attrName := range config.Config.LogAttrs {
		if attribute, found := strings.CutPrefix(attrName, "host."); found {
			if otelAttribute, exists := host.Attrs[attribute]; exists {
				attrs = append(attrs, apiLog.KeyValueFromAttribute(otelAttribute))
				continue
			}
		}
		if objType == MonService {
			if attribute, found := strings.CutPrefix(attrName, "service."); found {
				if otelAttribute, exists := service.Attrs[attribute]; exists {
					attrs = append(attrs, apiLog.KeyValueFromAttribute(otelAttribute))
					continue
				}
			}
		}
	}

	return

}

func (e IcingaEvent) OtelMetricAttributes() (attrs []attribute.KeyValue) {

	hostname, servicename, _ := e.MonObjectNames()
	attrs = append(attrs, semconv.HostName(hostname))
	attrs = append(attrs, semconv.ServiceName(servicename))
	attrs = append(attrs, attribute.String("object_type", e.MonObjectType().String()))

	attrs = append(attrs, e.ObjectMetricAttributes()...)

	return

}

func (e IcingaEvent) ObjectMetricAttributes() (attrs []attribute.KeyValue) {

	if len(config.Config.MetricAttrs) == 0 {
		return
	}

	hostname, servicename, _ := e.MonObjectNames()

	host := objectcache.GetHost(hostname)
	objType := e.MonObjectType()

	for _, attrName := range config.Config.MetricAttrs {
		if attribute, found := strings.CutPrefix(attrName, "host."); found {
			if otelAttribute, exists := host.Attrs[attribute]; exists {
				attrs = append(attrs, otelAttribute)
				continue
			}
		}
	}

	if objType == MonService {
		service := objectcache.GetService(hostname, servicename)
		for _, attrName := range config.Config.MetricAttrs {
			if attribute, found := strings.CutPrefix(attrName, "service."); found {
				if otelAttribute, exists := service.Attrs[attribute]; exists {
					attrs = append(attrs, otelAttribute)
					continue
				}
			}
		}
	}

	return

}




package objectcache

import (
	"log/slog"
	"gms/icinga2otel/internal/config"
	"gms/icinga2otel/internal/client"
	"net/http"
	"fmt"
	"encoding/json"
	"go.opentelemetry.io/otel/attribute"
	"sync"

)

type monObject struct {
	Name string `json:"name"`
	Attrs icingaAttrKeyValues `json:"attrs"`
}

type icingaHost struct {
	monObject
	Services map[string]monObject
}

type respResults struct {
	Results []monObject `json:"results"`
}

type objCacheStruct struct {
	hosts map[string]icingaHost
	mu sync.Mutex
}

var (
	objCache objCacheStruct = objCacheStruct{
		hosts: make(map[string]icingaHost),
	}
)

type icingaAttrKeyValues map[string]attribute.KeyValue

func (kvs *icingaAttrKeyValues) UnmarshalJSON(data []byte) error {

	if *kvs == nil {
		*kvs = make(icingaAttrKeyValues)
	}
	var jsonMap = make(map[string]interface{})

	if err := json.Unmarshal(data, &jsonMap); err == nil  {
		kvs.addAttrsFromMap(jsonMap, "")
	}

	return nil

}

// recursive processing of JSON attributes into Otel attribute map
func (kvs *icingaAttrKeyValues) addAttrsFromMap( jsonMap map[string]interface{}, keyTag string) {

	for key, val := range jsonMap {
		if key == "last_check_result" {
			continue
		}
		attrKey := keyTag + "." + key
		if keyTag == "" {
			attrKey = key
		}

		switch v := val.(type) {
		case int64:
			(*kvs)[attrKey] = attribute.Int64(attrKey, v)
		case float64:
			(*kvs)[attrKey] = attribute.Float64(attrKey, v)
		case bool:
			(*kvs)[attrKey] = attribute.Bool(attrKey, v)
		case string:
			(*kvs)[attrKey] = attribute.String(attrKey, v)
		case []interface{}: //need to decide on a type for the array
			if len(v) == 0 {
				continue
			}
			switch v[0].(type) {
			case int64:
				var slc []int64
				for _, convert := range v {
					if converted, ok := convert.(int64); ok {
						slc = append(slc, converted)
					}
				}
				(*kvs)[attrKey] = attribute.Int64Slice(attrKey, slc)
			case float64:
				var slc []float64
				for _, convert := range v {
					if converted, ok := convert.(float64); ok {
						slc = append(slc, converted)
					}
				}
				(*kvs)[attrKey] = attribute.Float64Slice(attrKey, slc)
			case bool:
				var slc []bool
				for _, convert := range v {
					if converted, ok := convert.(bool); ok {
						slc = append(slc, converted)
					}
				}
				(*kvs)[attrKey] = attribute.BoolSlice(attrKey, slc)
			case string:
				var slc []string
				for _, convert := range v {
					if converted, ok := convert.(string); ok {
						slc = append(slc, converted)
					}
				}
				(*kvs)[attrKey] = attribute.StringSlice(attrKey, slc)
			}

		case map[string]interface{}:
			kvs.addAttrsFromMap(v, attrKey)

		}
	}
}



func init() {
	Refresh()
}

func Refresh() {

	RefreshHosts("")
	RefreshServices("")

}

//Supply host nameFilter to refresh only one host. Emtpy refreshes all
func RefreshHosts(nameFilter string) {
	slog.Debug("Refreshing Host Objects Cache")

	objCache.mu.Lock()
	defer objCache.mu.Unlock()

	var hostData []monObject = retrieveObjects("hosts", nameFilter)

	for _, data := range hostData {
		if host, exists := objCache.hosts[data.Name]; exists {
			host.Attrs = data.Attrs
			objCache.hosts[data.Name] = host
		} else {
			objCache.hosts[data.Name] = icingaHost{
				monObject: monObject{
					Name: data.Name,
					Attrs: data.Attrs,
				},
				Services: make(map[string]monObject),
			}
		}
	}
	slog.Info("Refreshed Host Object Cache", "filter", nameFilter, "count", len(hostData))

}

//Supply name filter in form {host}!{service} to refresh only one service.  Empty refresh all.
func RefreshServices(nameFilter string) {
	slog.Debug("Refreshing Service Objects Cache")

	objCache.mu.Lock()
	defer objCache.mu.Unlock()

	var serviceData []monObject = retrieveObjects("services", nameFilter)

	for _, data := range serviceData {
		hostName := data.Attrs["host_name"].Value.AsString()
		serviceName := data.Attrs["name"].Value.AsString()

		if _, exists := objCache.hosts[hostName]; !exists {
			RefreshHosts(hostName)
		}

		if host, exists := objCache.hosts[hostName]; exists {

			if service, exists := host.Services[serviceName]; exists {
				service.Attrs = data.Attrs
				host.Services[serviceName] = service
			} else {
				host.Services[serviceName] = monObject{
					Name: data.Name,
					Attrs: data.Attrs,
				}
			}

		} else {
			slog.Error("Received an service but could not lookup host")
		}

	}
	slog.Info("Refreshed Service Object Cache", "filter", nameFilter, "count", len(serviceData))

}




func retrieveObjects(objType string, nameFilter string) (r []monObject) {

	var url string
	var results respResults


	url = fmt.Sprintf("https://%s/v1/objects/%s/%s", client.GetIcingaHost(), objType, nameFilter)

	client := client.GetIcingaHttpClient()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		slog.Error("Could not connect to Icinga Objects API", "error", err)
		return
	}

	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(config.Config.IcingaUser, config.Config.IcingaPass)

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("Could not connect to Icinga Objects API", "error", err)
		return
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		slog.Error("Could not decode JSON from Icinga Objects API", "error", err)
		return
	}


	r = results.Results
	return

}


//Get functions handle cache update if asked for object that is not in cache yet
func GetHost(hostName string) (r icingaHost) {

	var exists bool
	if r, exists = objCache.hosts[hostName]; exists {
		return
	}
	RefreshHosts(hostName)
	r = objCache.hosts[hostName]
	return
}

func GetService(hostName string, serviceName string) (r monObject) {

	var exists bool

	h := GetHost(hostName)
	if r, exists = h.Services[serviceName]; exists {
		return
	}
	RefreshServices(hostName + "!" + serviceName)
	h = GetHost(hostName)
	return h.Services[serviceName]
}


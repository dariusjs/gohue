package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"
)

// HueBridges holds the hue bridge objects
type HueBridges []HueBridge

// HueBridge structure for discovered hue bridges
type HueBridge struct {
	ID                string `json:"id"`
	Internalipaddress string `json:"internalipaddress"`
}

// HueResources storing hue resources
type HueResources struct {
	Config        map[string]interface{} `json:"config"`
	Scenes        map[string]interface{} `json:"scenes"`
	Schedules     map[string]interface{} `json:"schedules"`
	Sensors       map[string]HueSensor   `json:"sensors"`
	Resourcelinks map[string]interface{} `json:"resourcelinks"`
	Lights        map[string]interface{} `json:"lights"`
	Rules         map[string]interface{} `json:"rules"`
}

// HueSensor storing hue sensor objects
type HueSensor struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Config HueSensorConfig `json:"config"`
	State  HueSensorState  `json:"state"`
}

// HueSensorState storing hue sensor state
type HueSensorState struct {
	Temperature float64 `json:"temperature"`
	Lightlevel  float64 `json:"lightlevel"`
}

// HueSensorConfig storing hue sensor config
type HueSensorConfig struct {
	Battery float64 `json:"battery"`
}

func init() {
	// Log as JSON instead of the default ASCII formatter.
	log.SetFormatter(&log.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	log.SetLevel(log.DebugLevel)
}

func discoverHueBridges(hueAPIKey string, influxDbAddress string, hueDiscoveryURL string) HueBridges {
	var hueBridges HueBridges

	err := backoff.Retry(func() error {
		response, err := http.Get(hueDiscoveryURL)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Fatal(err)
		}

		err = json.Unmarshal([]byte(responseData), &hueBridges)
		if err != nil {
			log.Fatal(err)
		}
		return nil
	}, backoff.NewExponentialBackOff())

	if err != nil {
		log.Fatal(err)
	}
	log.WithFields(log.Fields{
		"HueBridges": hueBridges,
	}).Info("Discovered Hue Bridges")

	return hueBridges
}

func discoverHueSensors(hueBridges HueBridges, hueAPIKey string, influxDbAddress string) {
	for _, value := range hueBridges {
		bridgeAddress := value.Internalipaddress
		hueSensorURL := "http://" + bridgeAddress + "/api/" + hueAPIKey
		response, err := http.Get(hueSensorURL)
		if err != nil {
			log.Print(err)
		}
		defer response.Body.Close()

		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Print(err)
		}

		var hueResources HueResources
		err = json.Unmarshal([]byte(responseData), &hueResources)
		if err != nil {
			log.Print(err)
		}
		hueSensors := hueResources.Sensors

		for _, Value := range hueSensors {
			hueSensor := Value
			if err != nil {
				log.Print(err)
			}
			if hueSensor.Type == "ZLLTemperature" {
				hueSensor.Name = strings.ReplaceAll(hueSensor.Name, " ", "_")
				hueSensor.State.Temperature = hueSensor.State.Temperature / 100
				payload := "hue," + "name=" + fmt.Sprint(hueSensor.Name) + " temperature=" + fmt.Sprint(hueSensor.State.Temperature) + ",battery=" + fmt.Sprint(hueSensor.Config.Battery)
				log.Debug(payload)
				postToInflux(payload, influxDbAddress)
			}
			if hueSensor.Type == "ZLLLightLevel" {
				hueSensor.Name = strings.ReplaceAll(hueSensor.Name, " ", "_")
				lux := hueSensor.State.Lightlevel - 1
				lux = lux / 10000
				lux = math.Pow(10, lux)
				payload := "hue," + "name=" + fmt.Sprint(hueSensor.Name) + " lux=" + fmt.Sprint(lux) + ",battery=" + fmt.Sprint(hueSensor.Config.Battery)

				log.Debug(payload)
				postToInflux(payload, influxDbAddress)
			}
		}
	}
}

func postToInflux(payload string, influxDbAddress string) {
	err := backoff.Retry(func() error {
		response, err := http.Post(influxDbAddress, "application/octet-stream", bytes.NewBuffer([]byte(payload)))
		if err != nil {
			return err
		}
		log.Print(response)
		defer response.Body.Close()
		return nil
	}, backoff.NewExponentialBackOff())

	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	hueAPIKey := os.Getenv("HUE_API_KEY")
	influxDbAddress := os.Getenv("INFLUX_DB_ADDRESS")
	hueDiscoveryURL := "https://discovery.meethue.com/"

	webserver := http.NewServeMux()

	// Initial Discover
	hueBridges := discoverHueBridges(hueAPIKey, influxDbAddress, hueDiscoveryURL)
	discoverHueSensors(hueBridges, hueAPIKey, influxDbAddress)

	// Scheduled scan
	tick := time.Tick(5 * time.Minute)
	for range tick {
		discoverHueSensors(hueBridges, hueAPIKey, influxDbAddress)
	}
	err := http.ListenAndServe(":4000", webserver)
	log.Fatal(err)
}

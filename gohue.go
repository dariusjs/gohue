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
	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
)

type HueBridges []HueBridge

type HueBridge struct {
	Id                string `json:"id"`
	Internalipaddress string `json:"internalipaddress"`
}

type HueSensor struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Config HueSensorConfig `json:"config"`
	State  HueSensorState  `json:"state"`
}

type HueSensorState struct {
	Temperature float64 `json:"temperature"`
	Lightlevel  float64 `json:"lightlevel"`
}

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
	log.SetLevel(log.InfoLevel)
}

func discoverHueBridges(hue_api_key string, influx_db_address string, hueDiscoveryUrl string) HueBridges {
	var hueBridges HueBridges

	err := backoff.Retry(func() error {
		response, err := http.Get(hueDiscoveryUrl)
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

func discoverHueSensors(hueBridges HueBridges, hue_api_key string, influxDbAddress string) {
	for _, value := range hueBridges {
		bridgeAddress := value.Internalipaddress
		hueSensorUrl := "http://" + bridgeAddress + "/api/" + hue_api_key + "/sensors/"
		response, err := http.Get(hueSensorUrl)
		if err != nil {
			log.Print(err)
		}
		defer response.Body.Close()

		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Print(err)
		}

		var hueSensors map[string]interface{}
		json.Unmarshal([]byte(responseData), &hueSensors)
		err = json.Unmarshal([]byte(responseData), &hueSensors)
		if err != nil {
			log.Print(err)
		}

		for _, Value := range hueSensors {
			var hueSensor HueSensor
			err := mapstructure.Decode(Value, &hueSensor)
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
		fmt.Println(response)
		defer response.Body.Close()
		return nil
	}, backoff.NewExponentialBackOff())

	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	hueApiKey := os.Getenv("HUE_API_KEY")
	influxDbAddress := os.Getenv("INFLUX_DB_ADDRESS")
	hueDiscoveryUrl := "https://discovery.meethue.com/"

	webserver := http.NewServeMux()

	// Initial Discover
	hueBridges := discoverHueBridges(hueApiKey, influxDbAddress, hueDiscoveryUrl)
	discoverHueSensors(hueBridges, hueApiKey, influxDbAddress)

	// Scheduled scan
	tick := time.Tick(5 * time.Minute)
	for range tick {
		discoverHueSensors(hueBridges, hueApiKey, influxDbAddress)
	}
	err := http.ListenAndServe(":4000", webserver)
	log.Fatal(err)
}

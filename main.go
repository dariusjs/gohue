package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

type HueBridges []HueBridge

type HueBridge struct {
	Id                string `json:"id"`
	Internalipaddress string `json:"internalipaddress"`
}

type HueSensor struct {
	Name   string              `json:"name"`
	Type   string              `json:"type"`
	Config HueTempSensorConfig `json:"config"`
	State  HueTempSensorState  `json:"state"`
}

type HueTempSensorConfig struct {
	Battery float64 `json:"battery"`
}

type HueTempSensorState struct {
	Temperature float64 `json:"temperature"`
}

func discoverHueBridges(hue_api_key string, influx_db_address string, hueDiscoveryUrl string) HueBridges {
	response, err := http.Get(hueDiscoveryUrl)
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}

	var hueBridges HueBridges
	err = json.Unmarshal([]byte(responseData), &hueBridges)
	if err != nil {
		log.Fatal(err)
	}
	return hueBridges
}

func discoverHueSensors(hueBridges HueBridges, hue_api_key string) []HueSensor {
	var temperatureSensors []HueSensor
	for _, value := range hueBridges {
		bridgeAddress := value.Internalipaddress
		hueSensorUrl := "http://" + bridgeAddress + "/api/" + hue_api_key + "/sensors/"
		response, err := http.Get(hueSensorUrl)
		if err != nil {
			panic(err)
		}
		defer response.Body.Close()

		responseData, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Fatal(err)
		}

		var hueSensors map[string]interface{}
		json.Unmarshal([]byte(responseData), &hueSensors)
		err = json.Unmarshal([]byte(responseData), &hueSensors)
		if err != nil {
			log.Fatal(err)
		}

		for _, Value := range hueSensors {
			var hueSensor HueSensor
			err := mapstructure.Decode(Value, &hueSensor)
			if err != nil {
				log.Fatal(err)
			}
			if hueSensor.Type == "ZLLTemperature" {
				hueSensor.Name = strings.ReplaceAll(hueSensor.Name, " ", "_")
				hueSensor.State.Temperature = hueSensor.State.Temperature / 100
				temperatureSensors = append(temperatureSensors, hueSensor)
			}
		}
	}
	return temperatureSensors
}

func postToInflux(hueSensor HueSensor, influxDbAddress string) {
	payload := "hue," + "name=" + fmt.Sprint(hueSensor.Name) + " temperature=" + fmt.Sprint(hueSensor.State.Temperature) + ",battery=" + fmt.Sprint(hueSensor.Config.Battery)
	response, err := http.Post(influxDbAddress, "application/octet-stream", bytes.NewBuffer([]byte(payload)))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response)
}

func main() {
	hueApiKey := os.Getenv("HUE_API_KEY")
	influxDbAddress := os.Getenv("INFLUX_DB_ADDRESS")
	hueDiscoveryUrl := "https://discovery.meethue.com/"

	webserver := http.NewServeMux()

	hueBridges := discoverHueBridges(hueApiKey, influxDbAddress, hueDiscoveryUrl)
	hueTemperatureSensors := discoverHueSensors(hueBridges, hueApiKey)
	for _, hueSensor := range hueTemperatureSensors {
		postToInflux(hueSensor, influxDbAddress)
	}
	tick := time.Tick(5 * time.Minute)
	for range tick {
		hueBridges := discoverHueBridges(hueApiKey, influxDbAddress, hueDiscoveryUrl)
		hueTemperatureSensors := discoverHueSensors(hueBridges, hueApiKey)
		for _, hueSensor := range hueTemperatureSensors {
			postToInflux(hueSensor, influxDbAddress)
		}
	}
	err := http.ListenAndServe(":4000", webserver)
	log.Fatal(err)
}

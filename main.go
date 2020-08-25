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
)

type HueBridges []HueBridge

type HueBridge struct {
	Id                string `json:"id"`
	Internalipaddress string `json:"internalipaddress"`
}

type HueSensor struct {
	Name  string         `json:"name"`
	Type  string         `json:"type"`
	State HueSensorState `json:"state"`
}

type HueSensors struct {
	Id   string
	Data []HueSensor
}

type HueTemperatureSensor struct {
	Name         string `json:"name"`
	Temperature  string `json:"temperature"`
	BatteryState string `json:"battery"`
}

type HueSensorState struct {
	Temperature string `json:"temperature"`
	Lastupdated string `json:"lastupdated"`
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
	// responseString := string(responseData)
	// fmt.Println(responseString)

	var hueBridges HueBridges
	err = json.Unmarshal([]byte(responseData), &hueBridges)
	if err != nil {
		log.Fatal(err)
	}
	return hueBridges
}

func discoverHueSensors(bridgeAddress string, hue_api_key string) ([]string, []string) {
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
	var temperaturePayload []string
	var batteryPayload []string
	for Key, Value := range hueSensors {
		for _, value := range Value.(map[string]interface{}) {
			if value == "ZLLTemperature" {
				deviceName := strings.ReplaceAll(fmt.Sprint(hueSensors[Key].(map[string]interface{})["name"]), " ", "_")
				deviceTemperature := (hueSensors[Key].(map[string]interface{})["state"].(map[string]interface{})["temperature"]).(float64) / 100
				deviceBatteryState := hueSensors[Key].(map[string]interface{})["config"].(map[string]interface{})["battery"]

				temperaturePayload = append(temperaturePayload, "temperature,name="+deviceName+" value="+fmt.Sprint(deviceTemperature))
				batteryPayload = append(batteryPayload, "battery,name="+deviceName+" value="+fmt.Sprint(deviceBatteryState))
			}
		}
	}

	return temperaturePayload, batteryPayload
}

func postToInflux(hueBridges HueBridges, hueApiKey string, influxDbAddress string) {
	for _, value := range hueBridges {
		bridgeAddress := value.Internalipaddress
		temperaturePayload, batteryPayload := discoverHueSensors(bridgeAddress, hueApiKey)

		for _, value := range temperaturePayload {
			fmt.Println(value)
			response, err := http.Post(influxDbAddress, "application/octet-stream", bytes.NewBuffer([]byte(value)))
			if err != nil {
				panic(err)
			}
			fmt.Println(response)
		}
		for _, value := range batteryPayload {
			fmt.Println(value)
			response, err := http.Post(influxDbAddress, "application/octet-stream", bytes.NewBuffer([]byte(value)))
			if err != nil {
				panic(err)
			}
			fmt.Println(response)
		}
	}
}

func main() {
	hueApiKey := os.Getenv("HUE_API_KEY")
	influxDbAddress := os.Getenv("INFLUX_DB_ADDRESS")
	hueDiscoveryUrl := "https://discovery.meethue.com/"

	webserver := http.NewServeMux()
	tick := time.Tick(5 * time.Minute)
	for range tick {
		hueBridges := discoverHueBridges(hueApiKey, influxDbAddress, hueDiscoveryUrl)
		postToInflux(hueBridges, hueApiKey, influxDbAddress)
	}
	err := http.ListenAndServe(":4000", webserver)
	log.Fatal(err)
}

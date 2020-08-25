# gohue

#### To get an API key from the bridge
```
curl -d '{"devicetype:"application_name#device_name"}' --header "Content-Type: application/json" --request POST http://<bridge IP address>/api
```


#### Run the program
You will need to export an environment variable HUE_API_KEY with the API key extracted from above. Also a INFLUX_DB_ADDRESS is needed for the influxdb address. The program will then discover hue bridges and sensors within the network.

```
HUE_API_KEY=some_api_key cargo run  INFLUX_DB_ADDRESS=http://INFLUXDB_ADDRESS:8086/write?db=homestats gohue
```

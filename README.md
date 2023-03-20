# Welcome to the Sample HTTP Golang Server for Telemetry Data!
Three RESTful APIs are hosted on this server:

* `POST` request at `/temp`

* `GET` request at `/errors`

* `DELETE` request at `/errors`

## Core Features
* `POST` request at `/temp`:
	- `{"data": __data_string__}`  where `__data_string__` is format:
	- `__device_id__:__epoch_ms__:'Temperature':__temperature__`  where `__device_id__` is the device ID (int32)
	- `__epoch_ms__` is the timestamp in EpochMS (int64)
	- `__temperature__` is the temperature (float64) and `'Temperature'` is the exact string
	- Example `{"data": "365951380:1640995229697:'Temperature':58.48256793121914"}`
  
* `POST` response at `/temp`:
	* If for any reason the data string is not formatted correctly, return `{"error": "bad request"}` with a `400` status code
	- If the temperature is at or over 90, return `{"overtemp": true, "device_id": __device_id__, "formatted_time": __formatted_time__}`, where `__device_id__` is the device ID (int32) and `__formatted_time__` is the timestamp formatted to `%Y/%m/%d %H:%M:%S` otherwise return `{"overtemp": false}`

* `GET` request at `/errors` 
	* Return all data strings which have been incorrectly formatted. 
	* The response should be in the following format: `{"errors": [__error1__, __error2__] }` Where `__errorX__` is the exact data string received

* `DELETE` request at `/errors` 
	* clear the errors buffer.

## Manual Tests
* POST /temp - Over Temperature
```
curl -i --request POST \
  --url http://127.0.0.1:8080/temp \
  --header 'Content-Type: application/json' \
  --data '{"data": "365951380:1640995229697:'Temperature':110.48256793121914"}'

HTTP/1.1 200 OK
Content-Type: application/json
Date: Sun, 19 Mar 2023 19:14:10 GMT
Content-Length: 80

{"device_id":"365951380","formatted_time":"2021/12/31 16:00:29","overtemp":true}%    
```

* POST /temp - Good Temperature
```
curl -i --request POST \
  --url http://127.0.0.1:8080/temp \
  --header 'Content-Type: application/json' \
  --data '{"data": "365951380:1640995229697:'Temperature':10.48256793121914"}'
HTTP/1.1 200 OK
Content-Type: application/json
Date: Sun, 19 Mar 2023 19:14:42 GMT
Content-Length: 18

{"overtemp":false}%      
```

* POST /temp - Bad Request
```
 curl -i --request POST \
  --url http://127.0.0.1:8080/temp \
  --header 'Content-Type: application/json' \
  --data '{"data": "365951380:1640995229697:'TemperatureXX':110.48256793121914"}'
HTTP/1.1 400 Bad Request
Content-Type: text/plain; charset=utf-8
X-Content-Type-Options: nosniff
Date: Sun, 19 Mar 2023 18:55:50 GMT
Content-Length: 25

{"error": "bad request"}
```

* GET /errors
```
curl -i http://127.0.0.1:8080/errors
HTTP/1.1 200 OK
Content-Type: application/json
Date: Sun, 19 Mar 2023 18:55:55 GMT
Content-Length: 58

["365951380:40995229697:TemperatureXX:110.48256793121914"]
```

* DELETE /errors
```
curl --request DELETE  http://127.0.0.1:8080/errors
Then get /errors, all errors are cleared.
```

## Unit tests
```
Running tool: /Users/fan/sdk/go1.20/bin/go test -timeout 30s -run ^TestTempurature$ app/handler

ok  	app/handler	0.214s
```

## Additional Features
- Using a yaml config file to drive the application and no hardcoded endpoints in code.
- Unit-tests for http handler to cover several bad post body cases.
- Wrote a simple customized logger for easier debugging.
- This package uses a high performance HTTP router(https://github.com/julienschmidt/httprouter) which has better performance than the obsolete Gorilla Mux(https://github.com/gorilla/mux).
- The package uses in-memory storage with sync.Mutex to avoid read/write race conditions. In order to achieve a higher performance, a 3rd party in-memory storage like Redis can be considered.
- A docker file and a docker-compose file are included, making it possible to run on a containerized (K8s) environment to achieve higher performance.
- A README file is included, the package renders the markdown to be HTML format so it's easier to read.

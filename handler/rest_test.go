package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/require"
)

const (
	testURL          = "/test"
	testTopic        = "test topic"
	testVehicle      = "test vehicle"
	testTraceID      = "test traceID"
	testDeploymentID = "test deploymentID"
	testChecksum     = "test checksum"
)

// target types
const (
	errorURL = "/testTemp"
)

var logger = log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)

func getRequest(method, data string) *http.Request {
	b := map[string]string{
		"data": data,
	}

	body, err := json.Marshal(b)
	if err != nil {
		fmt.Println("failed to marshal body")
		return nil
	}

	req := httptest.NewRequest(method, errorURL, bytes.NewReader(body))
	req.Header.Set(ContentTypeKey, ContentTypeJSON)
	// RFC req.Header.Set("date", "Tue, 15 Nov 1994 08:12:31 GMT")
	req.Header.Set("date", "2023-03-19T16:16:57Z")
	return req
}

// GetTestResponseWithHandler returns a response from a handler
func GetTestResponseWithHandler(req *http.Request, method, url string,
	handler func(http.ResponseWriter, *http.Request, httprouter.Params)) *http.Response {
	w := httptest.NewRecorder()
	router := httprouter.New()
	router.Handle(method, url, handler)
	router.ServeHTTP(w, req)
	return w.Result()
}

func TestTempurature(t *testing.T) {
	type testData struct {
		name        string
		data        string
		goodRequest bool
		overTemp    bool
	}

	for _, td := range []testData{
		{name: "Good Req Good Temp", data: "365951380:1640995229697:'Temperature':10.48256793121914", goodRequest: true, overTemp: false},
		{name: "Good Req Bad Temp", data: "365951380:1640995229697:'Temperature':1000.48256793121914", goodRequest: true, overTemp: true},
		{name: "Bad Req 1", data: "365951380:1640995229697:'Tempeure':1000.48256793121914", goodRequest: false, overTemp: false},
		{name: "Bad Req 2", data: "365951380640995229697:'Tempeure':1000.48256793121914", goodRequest: false, overTemp: false},
		{name: "Bad Req 3", data: "365951380:1640995229697:'Tempeure':sd.48256793121914", goodRequest: false, overTemp: false},
	} {
		r := getRequest(http.MethodPost, td.data)
		PostTempHandler := PostTempHandler(logger)
		resp := GetTestResponseWithHandler(r, http.MethodPost, errorURL, PostTempHandler)
		defer resp.Body.Close()
		if td.goodRequest {
			payload, _ := io.ReadAll(resp.Body)
			payloadStr := string(payload)
			require.Equal(t, http.StatusOK, resp.StatusCode, td.name+"should pass")
			if td.overTemp {
				require.True(t, strings.Contains(payloadStr, ":true"), td.name+"should pass")
			} else {
				require.True(t, strings.Contains(payloadStr, "false"), td.name+"should pass")

			}
		} else {
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, td.name+"should fail")
		}
	}

	// Get errors
	r := getRequest(http.MethodGet, "")
	PostTempHandler := GetErrorsHandler(logger)
	resp := GetTestResponseWithHandler(r, http.MethodGet, errorURL, PostTempHandler)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get errrors")
	respB, _ := io.ReadAll(resp.Body)
	var params map[string]string
	_ = json.Unmarshal(respB, &params)
	dataStr, ok := params["error"]
	require.True(t, ok, "should contain 'error'")
	if len(dataStr) == 0 {
		errArr := strings.Split(dataStr, ",")
		require.Equal(t, 3, len(errArr), "should have 3 data")
	}
	// Delete errors
	d := getRequest(http.MethodDelete, "")
	dHandler := DeleteHandler(logger)
	rDel := GetTestResponseWithHandler(d, http.MethodDelete, errorURL, dHandler)
	require.Equal(t, http.StatusOK, rDel.StatusCode, "delete errrors")

	// Get errors again, should have 0 after deletion
	resp = GetTestResponseWithHandler(r, http.MethodGet, errorURL, PostTempHandler)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get errrors")
	respB, _ = io.ReadAll(resp.Body)
	_ = json.Unmarshal(respB, &params)
	dataStr, ok = params["error"]
	require.Equal(t, "[]", dataStr, "should have 0 errors")
}

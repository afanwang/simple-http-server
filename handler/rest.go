package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/julienschmidt/httprouter"
)

// Error codes
const (
	ContentTypeKey     = "Content-Type"
	ContentTypeJSON    = "application/json"
	ContentTypeHTML    = "text/html"
	InvalidContentType = "invalid content type"
	TempThreshold      = 90.0
	// `%Y/%m/%d %H:%M:%S`
	TimeLayout = "2006/01/02 15:04:05"
)

// levels tracks how deep we are in a heading "structure"
var levels []int

func hasLevels() bool {
	return len(levels) > 0
}

func lastLevel() int {
	if hasLevels() {
		return levels[len(levels)-1]
	}
	return 0
}

func popLevel() int {
	level := lastLevel()
	levels = levels[:len(levels)-1]
	return level
}

func pushLevel(x int) {
	levels = append(levels, x)
}

var reID = regexp.MustCompile(`\s+`)

// renderSections catches an ast.Heading node, and wraps the node
// and its "children" nodes in <section>...</section> tags; there's no
// real hierarchy in Markdown, so we make one up by saying things like:
// - H2 is a child of H1, and so forth from 1 → 2 → 3 ... → N
// - an H1 is a sibling of another H1
func renderSections(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
	openSection := func(level int, id string) {
		w.Write([]byte(fmt.Sprintf("<section id=\"%s\">\n", id)))
		pushLevel(level)
	}
	closeSection := func() {
		w.Write([]byte("</section>\n"))
		popLevel()
	}

	if _, ok := node.(*ast.Heading); ok {
		level := node.(*ast.Heading).Level
		if entering {
			// close heading-sections deeper than this level; we've "come up" some number of levels
			for lastLevel() > level {
				closeSection()
			}

			txtNode := node.GetChildren()[0]
			if _, ok := txtNode.(*ast.Text); !ok {
				panic(fmt.Errorf("expected txtNode to be *ast.Text; got %T", txtNode))
			}
			headTxt := string(txtNode.AsLeaf().Literal)
			id := strings.ToLower(reID.ReplaceAllString(headTxt, "-"))

			openSection(level, id)
		}
	}

	// at end of document
	if _, ok := node.(*ast.Document); ok {
		if !entering {
			for hasLevels() {
				closeSection()
			}
		}
	}

	// continue as normal
	return ast.GoToNext, false
}

// EpochStrToFormatted converts epoch string to formatted string.
func EpochStrToFormatted(epochStr string) (string, error) {
	epoch, err := strconv.ParseInt(epochStr, 10, 64)
	if err != nil || epoch == 0 {
		return "", errors.New("Invalid utime for conversion")
	}

	t := time.Unix(0, int64(epoch)*int64(time.Millisecond))

	return t.Format(TimeLayout), nil
}

// Handler is the interface for all handlers.
func errorHandler(
	errMsg string,
	reasonErrMsg error,
	httpErrorStatus int,
	traceID string,
	log *log.Logger,
	w http.ResponseWriter,
) {
	var logError = errMsg
	if reasonErrMsg != nil {
		logError = fmt.Sprintf("%s Reason: %v", errMsg, reasonErrMsg)
	}
	log.Printf("Err: %s", logError)
	http.Error(w, errMsg, httpErrorStatus)
}

// ErrorHandler prints the error message and returns the error status.
func ErrorHandler(errMsg string, reasonErrMsg error, httpErrorStatus int,
	log *log.Logger, w http.ResponseWriter) {
	var logError = errMsg
	if reasonErrMsg != nil {
		logError = fmt.Sprintf("%s Reason: %v", errMsg, reasonErrMsg)
	}
	log.Printf("Error: %s", logError)
	http.Error(w, errMsg, httpErrorStatus)
}

// PanicHandler prints the stack trace and returns the error status.
func panicHandler(w http.ResponseWriter, r *http.Request, err interface{}) {
	debug.PrintStack()
	w.WriteHeader(http.StatusInternalServerError)
}

// NewRouter creates a new router
func NewRouter() *httprouter.Router {
	router := httprouter.New()
	router.PanicHandler = panicHandler
	return router
}

// NewServer creates a new HTTP server
func NewServer(port int, router http.Handler) *http.Server {
	server := http.Server{
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
	}
	return &server
}

type OverTempResponse struct {
	DeviceID      string `json:"device_id"`
	FormattedTime string `json:"formatted_time"`
	OverTemp      bool   `json:"overtemp"`
}

type NormalTempResponse struct {
	OverTemp bool `json:"overtemp"`
}

var myErrorStrings = errorStrings{
	errors: make([]string, 0),
}

type errorStrings struct {
	sync.Mutex
	errors []string
}

func (e *errorStrings) Push(str string) {
	e.Lock()
	defer e.Unlock()
	e.errors = append(e.errors, str)
}

func (e *errorStrings) Get() []string {
	e.Lock()
	defer e.Unlock()
	return e.errors
}

func (e *errorStrings) Len() int {
	e.Lock()
	defer e.Unlock()
	return len(e.errors)
}

func (e *errorStrings) Clear() {
	e.Lock()
	defer e.Unlock()
	e.errors = []string{}
}

// PostTempHandler is the handler for POST /temp
// Sample request body:
// `{"data": __data_string__}`
// - `__device_id__:__epoch_ms__:'Temperature':__temperature__`
// - `__device_id__` is the device ID (int32)
// - `__epoch_ms__` is the timestamp in EpochMS (int64)
// - `__temperature__` is the temperature (float64)
// - Example `{"data": "365951380:1640995229697:'Temperature':58.48256793121914"}`
func PostTempHandler(log *log.Logger) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		log.Printf("Received PostTemp request: %+v", r)

		if r.Header.Get(ContentTypeKey) != ContentTypeJSON {
			ErrorHandler("Error: request header missing "+ContentTypeKey,
				errors.New(InvalidContentType), http.StatusBadRequest, log, w)
			return
		}
		badReqRet := `{"error": "bad request"}`
		var params map[string]string
		var err error
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&params); err != nil {
			ErrorHandler(badReqRet,
				err, http.StatusBadRequest, log, w)
			b, _ := io.ReadAll(r.Body)
			if b != nil {
				myErrorStrings.Push(string(b))
			}
			return
		}

		data, ok := params["data"]
		if !ok || len(data) == 0 {
			ErrorHandler(badReqRet,
				errors.New("Got empty data"),
				http.StatusBadRequest, log, w)
			myErrorStrings.Push(data)
			return
		}

		//  Check temp range
		var tempString string
		var containsTemp bool
		if strings.Contains(data, ":Temperature:") {
			containsTemp = true
			tempString = ":Temperature:"
		}

		if strings.Contains(data, ":'Temperature':") {
			containsTemp = true
			tempString = ":'Temperature':"
		}

		if strings.Contains(data, `":\'Temperature:\'"`) {
			containsTemp = true
			tempString = ":'Temperature':"
		}
		if !containsTemp {
			ErrorHandler(badReqRet,
				errors.New("No temperature keyword"),
				http.StatusBadRequest, log, w)
			myErrorStrings.Push(data)
			return
		}

		fields := strings.Split(data, tempString)
		if len(fields) != 2 {
			ErrorHandler(badReqRet,
				errors.New("Wrong format temp fields"),
				http.StatusBadRequest, log, w)
			myErrorStrings.Push(data)
			return
		}

		deviceFields := strings.Split(fields[0], ":")
		if len(deviceFields) != 2 {
			ErrorHandler(badReqRet,
				errors.New("Wrong format: device fields"),
				http.StatusBadRequest, log, w)
			myErrorStrings.Push(data)
			return
		}

		deviceID := deviceFields[0]
		formatT, errF := EpochStrToFormatted(deviceFields[1])
		if errF != nil {
			ErrorHandler(badReqRet,
				errors.New("Wrong format: epoch time"),
				http.StatusBadRequest, log, w)
			myErrorStrings.Push(data)
			return
		}
		temp, error1 := strconv.ParseFloat(fields[1], 64)
		if error1 != nil {
			ErrorHandler(badReqRet,
				errors.New("Wrong format: parse float"),
				http.StatusBadRequest, log, w)
			myErrorStrings.Push(data)
			return
		}

		var respData []byte
		if temp > TempThreshold {
			// return `{"overtemp": true, "device_id": __device_id__, "formatted_time": __formatted_time__}`,
			resp := OverTempResponse{
				DeviceID:      deviceID,
				FormattedTime: formatT,
				OverTemp:      true,
			}
			respData, err = json.Marshal(resp)
			if err != nil {
				ErrorHandler(fmt.Sprintf("HTTP 500: Error while marshalling response"),
					err, http.StatusInternalServerError, log, w)
				return
			}
		} else {
			// return `{"overtemp": false}`
			resp := NormalTempResponse{
				OverTemp: false,
			}
			respData, err = json.Marshal(resp)
			if err != nil {
				ErrorHandler(fmt.Sprintf("HTTP 500: Error while marshalling response"),
					err, http.StatusInternalServerError, log, w)
				return
			}
		}
		w.Header().Set(ContentTypeKey, ContentTypeJSON)
		respCode, respErr := w.Write(respData)
		if respErr != nil {
			log.Printf("Error writing response (%d): url: %s, error: %s",
				respCode, r.URL.String(), respErr)
			return
		}
		log.Printf("Request succeeded: %d: %s", respCode, respData)
	}
}

func GetErrorsHandler(log *log.Logger) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		errStrs := myErrorStrings.Get()
		errStr, _ := json.Marshal(errStrs)
		restMap := map[string]string{
			"error": string(errStr),
		}
		respData, marErr := json.Marshal(restMap)
		if marErr != nil {
			errMsg := fmt.Sprintf(
				"Error performing Marshal indent for errorString %v: %v",
				errStrs, marErr,
			)
			log.Printf("Err %s", errMsg)
		}
		log.Printf("Received Get Errors request: %+v", r)

		if respData == nil {
			ErrorHandler("Json marshal failed",
				marErr, http.StatusInternalServerError, log, w)
			return
		}

		w.Header().Set(ContentTypeKey, ContentTypeJSON)
		respCode, respErr := w.Write(respData)
		if respErr != nil {
			log.Printf(
				"Error writing response (%d): url: %s, error: %s",
				respCode, r.URL.String(), respErr)
			return
		}
		log.Printf("Request succeeded: %d: %s", respCode, respData)
	}
}

// GetReadmeHandler returns the README.md file in HTML format
func GetReadmeHandler(log *log.Logger) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		log.Printf("Received Get Readme request: %+v", r)
		lines, err := ioutil.ReadFile("README.md")
		if err != nil {
			ErrorHandler("Error reading README.md",
				err, http.StatusInternalServerError, log, w)
			return
		}
		// md := strings.Join(lines, "\n")

		opts := html.RendererOptions{
			Flags:          html.CommonFlags,
			RenderNodeHook: renderSections,
		}
		renderer := html.NewRenderer(opts)
		w.Header().Set(ContentTypeKey, ContentTypeHTML)
		html := markdown.ToHTML(lines, nil, renderer)
		respCode, respErr := w.Write(html)
		if respErr != nil {
			log.Printf(
				"Error writing response (%d): url: %s, error: %s",
				respCode, r.URL.String(), respErr)
			return
		}
		log.Printf("Request succeeded: %d: %s", respCode, html)
	}
}

// ErrorHandler is a helper function to handle errors
func DeleteHandler(log *log.Logger) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		log.Printf("Received Get Errors request: %+v", r)
		len := myErrorStrings.Len()
		myErrorStrings.Clear()

		respData, marErr := json.Marshal("Success: Deleted " + strconv.Itoa(len) + " errors")
		if marErr != nil {
			errMsg := fmt.Sprintf(
				"Error Marshal indent for errorString len %d: %v",
				len, marErr,
			)
			log.Printf("Err %s", errMsg)
		}
		if respData == nil {
			ErrorHandler("No response can be returned",
				marErr, http.StatusInternalServerError, log, w)
			return
		}

		// w.Header().Set(ContentTypeKey, ContentTypeJSON)
		log.Printf("Delete Request Succeeded")
	}
}

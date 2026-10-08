package router_test

import (
	"math/rand"
	"net/http"
	"sort"
	"strings"
)

/******************************************************************************
 *                               Handlers
 ******************************************************************************/

// RouteAndQueryHandler accepts a string indicating the path of the route
// initially added to the router (of the form "/path/to/file")
// and writes to the response a summary of the request that was made,
// in the following format:
// <METHOD> <PATH>
// <RAWQUERY>
func RouteAndQueryHandler(route string) http.HandlerFunc {
	return func(response http.ResponseWriter, req *http.Request) {
		result := NewResult(req.Method, route, req.URL.RawQuery)
		out := result.String()
		response.Write([]byte(out))
	}
}

// Print the route only, ignoring the RawQuery
func RouteHandler(route string) http.HandlerFunc {
	return func(response http.ResponseWriter, req *http.Request) {
		result := NewResult(req.Method, route, "")
		out := result.String()
		response.Write([]byte(out))
	}
}

// OverwrtiteHandler should never be called.
// It exists only to be overwritten by another handler later.
func OverwriteHandler(response http.ResponseWriter, req *http.Request) {
	response.Write([]byte(NotOverwritten))
}

/******************************************************************************
 *                             ResponseWriters
 ******************************************************************************/
func NewHeader() http.Header {
	return make(map[string][]string)
}

// Store info inside struct's fields
// WARNING: The status code will persist between queries unless manually cleared!
type SelfWriter struct {
	status int
	out    string
}

func (dw *SelfWriter) Header() http.Header {
	return NewHeader()
}

func (dw *SelfWriter) Write(msg []byte) (n int, err error) {
	dw.out = (string(msg))
	return 0, nil
}

func (dw *SelfWriter) WriteHeader(status int) {
	dw.status = status
}

func (dw *SelfWriter) Clear() {
	dw.status = 0
	dw.out = ""
}

/******************************************************************************
 *                            Helper Functions
 ******************************************************************************/

// Create a new request with specified method and path, and default other fields
func NewRequest(method string, path string) (req *http.Request) {
	path = strings.Trim(path, "/")
	urlStr := "http://localhost:8080/" + path
	req, _ = http.NewRequest(method, urlStr, nil)
	return
}

// Generate a random a-zA-Z0-9 string of fixed length
func RandomString(n int) string {
	alphabet := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]rune, n)
	for i := 0; i < n; i++ {
		b[i] = rune(alphabet[rand.Intn(len(alphabet))])
	}
	return string(b)
}

// Sort an array of ints in place and return the largest
func Max(ns []int) int {
	sort.Sort(sort.Reverse(sort.IntSlice(ns)))
	return ns[0]
}

// Sort an array of ints in place and return the smallest
func Min(ns []int) int {
	sort.Ints(ns)
	return ns[0]
}

/*******************************************************************************
* A collection of tests for assignment 2: HTTP Router
*
* This directory is a package of the assignment module, so it is graded by
* running, from the root of the (student or reference) repository:
*       > go test -v ./grader
*
* To grade a student repo, copy this grader/ directory into the root of that
* repo and run the same command there.
*******************************************************************************/

package router_test

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"testing"

	//
//	"cos316.princeton.edu/assignment2/grader/benchmark"

	"cos316.princeton.edu/assignment2/http_router"
)

/******************************************************************************
 *                        Constants
 ******************************************************************************/
// Standard HTTP Methods
const (
	Get    = "GET"
	Post   = "POST"
	Put    = "PUT"
	Delete = "DELETE"
)

// Common strings that can be returned as test results
const (
	Pass = "PASS"
	Fail = "FAIL"
)

// Possible error strings that can be returned during testing
const (
	NotFound       = "404 page not found"
	NotOverwritten = "Handler was not correctly overwritten!"
)

// Map error strings we expect to encounter to their codes
// Not very useful if NotFound is the only error, but this will help
// if we ever decide to make students implement more.
var ErrorCodes = map[string]int{
	NotFound: 404,
}

/******************************************************************************
 *                     Error Messages & Test Failures
 ******************************************************************************/
// What to print on test failure
// $1 = list of routes currently in the router
// $2 = single request that was issued to make this test fail
// $3 = additional info about why the test failed; what was expected
var testFailMessage = `

************ TEST FAILED ************
Routes added to student router:
%s
Request issued to student router:
%s
%s

`

// ------------------ Reasons for test failures ------------------------------
// Message explaining that actual route differs from expected route
// $1 = ["use route" | "respond" if 404 NotFound]
// $3 = ["used route" | "responded" if 404 NotFound]
var testFailWrongRoute = `
Expected student router to %s:
- %s
but instead student router %s:
- %s
`

// Message explaining that actual rawquery and expected rawquery differ
var testFailWrongRawQuery = `
Expected student router to set RawQuery to:
- "%s"
but instead student router set RawQuery to:
- "%s"
`

var testPanicMessage = `
Go panicked while executing student code!

Error: %s
Stacktrace:
%s

`

// explainFailure() takes in an expected and a received, and
// explains why the test failed by enumerating differences between expected
// and received
func ExplainFailure(expected, received Result) string {
	var b strings.Builder

	expRoute := expected.RouteString()
	gotRoute := received.RouteString()

	expAction := "use route"
	gotAction := "used route"

	if expRoute == NotFound {
		expAction = "respond"
	}
	if gotRoute == NotFound {
		gotAction = "responded"
	}

	if gotRoute == NotOverwritten {
		expAction = "overwrite handler for route"
		gotAction = "used the old handler"
	}

	if expRoute != gotRoute {
		errStr := fmt.Sprintf(testFailWrongRoute, expAction, expRoute, gotAction, gotRoute)
		b.WriteString(errStr)
	}

	expQuery := expected.Values.String()
	gotQuery := received.Values.String()

	if expQuery != gotQuery {
		errStr := fmt.Sprintf(testFailWrongRawQuery, expQuery, gotQuery)
		b.WriteString(errStr)
	}

	if b.Len() == 0 {
		// This shouldn't happen
		return "Unable to provide further information on test failure."
	}

	return b.String()
}

/******************************************************************************
 *                             Type Defs
 ******************************************************************************/

// ------------------------- QueryValues -------------------------------------
// QueryParams represents an unordered set of key-value pairs, with each
// key possibly having several values. This is a representation of a URL's
// RawQuery attribute
type QueryValues struct {
	values url.Values
}

func NewQueryValues(rawQuery string) QueryValues {
	vals, _ := url.ParseQuery(rawQuery)
	return QueryValues{vals}
}

// String returns a string representation of the query values by converting it
// to a RawQuery string alphabetized by key, then by value
func (q QueryValues) String() string {
	return q.values.Encode()
}

// Equals returns true iff p and q have exactly the same set of keys,
// and each key has the exact set set of values.
func (p QueryValues) Equals(q QueryValues) bool {
	return p.String() == q.String()
}

// ---------------------------- Result -----------------------------------
// Result represents the high-level "outcome" of a routing operation,
// storing the route that was taken, and the query parameters that were passed
// Route will be equal to NotFound if there was no route to be found in router
// and equal to NotOverwritten if the router's handler should have been overwritten by another
// Otherwise Route is the path of the route, with captures left as-is:
// e.g. "/path/to/route", "/path/to/:file"
type Result struct {
	Method string
	Route  string
	Values QueryValues
}

func NewResult(method, route, rawQuery string) Result {
	values := NewQueryValues(rawQuery)

	if route == NotFound || route == NotOverwritten {
		return Result{"", route, values}
	}

	// Trim slashes from path, add leading slash (so it looks nice)
	route = strings.Trim(route, "/")
	route = "/" + route
	return Result{method, route, values}
}

// NewResultFromString constructs a new result from a string of the format
// produced by Result.String()
func NewResultFromString(s string) Result {
	if s == NotOverwritten {
		return NewResult("", NotOverwritten, "")
	}

	lines := strings.Split(s, "\n")
	if len(lines) != 2 {
		log.Panicf("Expected method, path, and rawquery but found only '%s'", s)
	}

	// Get the index of the first space (separates method from path)
	rtLine := lines[0]
	i := strings.Index(rtLine, " ")
	if i == -1 {
		log.Panicf("Expected a method and path but found only '%s'", rtLine)
	}
	method := rtLine[:i]
	path := rtLine[i+1:]

	if rtLine == NotFound {
		method = ""
		path = NotFound
	}

	return NewResult(method, path, lines[1])
}

// String() returns a string representation of Result.
// It must be in the exact same format as RouteAndQueryHandler's output so the
// two can be compared for equality
func (res Result) String() string {
	return fmt.Sprintf("%s\n%s", res.RouteString(), res.Values)
}

// If the Result is an error like NotFound or NotOverwritten, return that
// error as a string. Otherwise, return the Result as a string of the form:
// "<METHOD> <ROUTE>"
func (res Result) RouteString() string {
	if res.Route == NotFound || res.Route == NotOverwritten {
		return res.Route
	}
	return fmt.Sprintf("%s %s", res.Method, res.Route)
}

func (res Result) NotFound() bool {
	return res.Route == NotFound
}

// ----------------------------- Request -----------------------------------
// Request stores the binding from method and path to an expected result,
// so that tests can compare the actual and expected results of the Request
type Request struct {
	method   string
	path     string
	expected Result
}

// String() returns a string representation of the Request, excluding its
// expected value
func (req Request) String() string {
	var b strings.Builder
	b.WriteString(req.method)
	if b.Len() != 0 {
		b.WriteByte(' ')
	}
	b.WriteString(req.path)
	return b.String()
}

// ----------------------------- Route -----------------------------------
// A Route stores the binding from a method and path to an HTTP handler
type Route struct {
	method  string
	path    string
	handler http.HandlerFunc
}

// String() converts a single Route into a string for pretty printing
func (rt Route) String() string {
	return fmt.Sprintf("Method: %-12s Path: %s\n", rt.method, rt.path)
}

// ----------------------------- Routes -----------------------------------
// Routes is an alias for []Route to allow it to act as a receiver
type Routes []Route

// String() converts an array of Routes into a string for pretty printing
func (rts Routes) String() string {
	var b strings.Builder
	for _, rt := range rts {
		b.WriteString("  - ")
		b.WriteString(rt.String())
	}
	return b.String()
}

// ----------------------------- Test -----------------------------------
// A Test stores a list of routes to add to a router, and a list of requests
// that should subsequently be made to that router to verify its expected behavior
type Test struct {
	routes   []Route   // A list of routes to be added to router
	requests []Request // A list of requests to be queried to router
	simple   bool      // If true, ignore routes & test each request in a new router
}

func NewTest(routes []Route, requests []Request) Test {
	return Test{routes, requests, false}
}

/******************************************************************************
 *                        Route & Request Makers
 ******************************************************************************/

// MakeRoutes takes a list of methods, paths, and creates
// a list of routes rts where rts[i] = Route{methods[i], paths[i], handlers[i]}.
// If any of the lists is exhausted before all are complete, the final slice
// element will be repeated as needed.
func MakeRoutes(methods []string, paths []string, handlers []http.HandlerFunc) Routes {
	n := Max([]int{len(methods), len(paths), len(handlers)})
	rts := make([]Route, n)
	for i := 0; i < n; i++ {
		m := Min([]int{i, len(methods) - 1})
		p := Min([]int{i, len(paths) - 1})
		h := Min([]int{i, len(handlers) - 1})
		rt := Route{methods[m], paths[p], nil}

		// If handlers is nil or empty, use the default handler.
		if h == -1 || handlers[h] == nil {
			rt.handler = RouteAndQueryHandler(rt.path)
		} else {
			rt.handler = handlers[h]
		}
		rts[i] = rt
	}
	return rts
}

// MakeRequests takes a list of methods, paths, and expected values, and creates
// a list of requests rqs such that:
// rqs[i] = Request{methods[i], paths[i], NewResult(expRts[i], expRQs[i])}
// If any of the lists is exhausted before all are complete, the final slice
// element will be repeated as needed.
func MakeRequests(methods []string, paths []string, expRts []string, expRQs []string) []Request {
	n := Max([]int{len(methods), len(paths), len(expRts), len(expRQs)})
	rqs := make([]Request, n)
	for i := 0; i < n; i++ {
		m := Min([]int{i, len(methods) - 1})
		p := Min([]int{i, len(paths) - 1})
		r := Min([]int{i, len(expRts) - 1})
		q := Min([]int{i, len(expRQs) - 1})

		// if expRQs is empty or nil, just use the empty string
		expRQ := ""
		if q != -1 {
			expRQ = expRQs[q]
		}

		expected := NewResult(methods[m], expRts[r], expRQ)
		rqs[i] = Request{methods[m], paths[p], expected}
	}
	return rqs
}

/******************************************************************************
 *                           Testing Helpers
 ******************************************************************************/
// CatchPanic prints stack trace and an error message if student code panics
func CatchPanic(t *testing.T, test Test, rq *Request) {
	if e := recover(); e != nil {
		trace := debug.Stack()
		panicMsg := fmt.Sprintf(testPanicMessage, e, trace)
		rtMsg := Routes(test.routes).String()
		rqMsg := (*rq).String()
		if rqMsg == "" {
			rqMsg = "None - router panicked before requests could be made."
		}
		t.Errorf(testFailMessage, rtMsg, rqMsg, panicMsg)
		t.FailNow()
	}
}

func RunTest(t *testing.T, test Test) {
	var rq Request                 // declared here so CatchPanic can access it
	defer CatchPanic(t, test, &rq) // Catch panics raised by student code so all tests finish

	currentRoutes := make([]Route, 0) // routes currently in the router
	router := http_router.NewRouter()
	w := new(SelfWriter) // Consider moving this inside the request loop

	// Add all routes
	for _, rt := range test.routes {
		router.AddRoute(rt.method, rt.path, rt.handler)
		currentRoutes = append(currentRoutes, rt)
	}

	// Check all requests
	for _, rq = range test.requests {
		subtestName := fmt.Sprintf("%s&%s", rq.method, rq.path)
		t.Run(subtestName, func(t *testing.T) {
			defer CatchPanic(t, test, &rq) // Catch panics raised by student code so all tests finish

			// For simple tests, check rq against a router whose only route is rq
			// capture not supported for simple tests
			if test.simple {
				rt := Route{rq.method, rq.path, nil}
				rt.handler = RouteAndQueryHandler(rt.path)

				router = http_router.NewRouter()
				router.AddRoute(rt.method, rt.path, rt.handler)
				currentRoutes = []Route{rt}

				rq.expected = NewResult(rt.method, rt.path, "")
			}

			// Route current request, and ensure student router provides expected output
			req := NewRequest(rq.method, rq.path)
			router.ServeHTTP(w, req)
			if w.out != rq.expected.String() {
				// For errors we only require the expected status code to match, not the content
				if expCode, didExpectErr := ErrorCodes[rq.expected.Route]; didExpectErr && w.status == expCode {
					w.Clear() // Warning! Tests will silently break if this is removed
				} else {
					// The response was unexpected - test fails
					explanation := ExplainFailure(rq.expected, NewResultFromString(w.out))
					t.Errorf(testFailMessage, Routes(currentRoutes), rq, explanation)
					return // halt subtest
				}
			}
		})
	}
}

/******************************************************************************
 *                               TESTS
 ******************************************************************************/

/*-----------------------------------------------------------------------------
 *                          Testing Methods
 -----------------------------------------------------------------------------*/

func TestStandardMethods(t *testing.T) {
	// desc := "Check requests that use standard HTTP methods"

	methods := []string{Get, Post, Put, Delete}
	paths := []string{"/examplepath"}
	routes := MakeRoutes(methods, paths, nil)
	requests := MakeRequests(methods, paths, paths, nil)

	test := NewTest(routes, requests)
	test.simple = true
	RunTest(t, test)
}

// Test that method names aren't hardcoded, and arbitrary names would work
func TestArbitraryMethods(t *testing.T) {
	// desc := "Check requests that use arbitrary alphanumeric methods"

	rand.Seed(316316316) // Same random strings for everyone
	n := 10
	methods := make([]string, n)
	for i := 0; i < n; i++ {
		m := rand.Intn(8) + 1
		methods[i] = RandomString(m)
	}
	paths := []string{"/examplepath"}
	routes := MakeRoutes(methods, paths, nil)
	requests := MakeRequests(methods, paths, paths, nil)

	test := NewTest(routes, requests)
	test.simple = true
	RunTest(t, test)
}

// Test that method names aren't case sensitive
func TestMethodCaseSensitivity(t *testing.T) {
	// desc := "Check that request methods are not case-sensitive"

	methods := []string{"GET", "get", "gEt", "GeT", "GEt", "gET"}
	paths := []string{"/examplepath"}
	routes := MakeRoutes(methods, paths, nil)
	requests := MakeRequests(methods, paths, paths, nil)

	test := NewTest(routes, requests)
	test.simple = true
	RunTest(t, test)
}

// Test that when several methods cover a given path, the correct one is used
func TestOverlappingMethods(t *testing.T) {
	// desc := "Check routes that use the same path but different request methods"

	methods := []string{Get, Post, Put, Delete}
	paths := []string{"/examplepath"}

	routes := MakeRoutes(methods, paths, nil)
	requests := MakeRequests(methods, paths, paths, nil)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

/*-----------------------------------------------------------------------------
 *                              Testing Paths
 -----------------------------------------------------------------------------*/

// Make sure common paths weren't hardcoded (ie check paths that were never added)
func TestHardcodedPaths(t *testing.T) {
	// desc := "Check that common routes were not hard-coded in an empty router"

	getPaths := []string{
		"/", // TODO: Make sure this won't break the tests or routers (it should get filtered to "")
		"/index.html",
		"/feed",
		"/threads",
		"/threads/thread",
		"/threads/0",
		"/threads/1",
		"/users/user/recent",
		"/users/amit/recent",
		"/users/kap/recent",
		"/users/will/recent",
	}
	getRequests := MakeRequests([]string{Get}, getPaths, []string{NotFound}, nil)

	postPaths := []string{
		"/",
		"/index.html",
		"/feed",
		"/threads/thread",
		"/threads/0",
		"/threads/1",
		"/threads/2",
		"/users/new",
		"/users/user/follow",
		"/users/amit/follow",
		"/users/kap/follow",
		"/users/will/follow",
	}
	postRequests := MakeRequests([]string{Post}, postPaths, []string{NotFound}, nil)

	requests := make([]Request, len(getRequests)+len(postRequests))
	copy(requests, getRequests)
	copy(requests[len(getRequests):], postRequests)

	test := NewTest([]Route{}, requests)
	RunTest(t, test)
}

// Test that paths work when they are multiple directories deep.
func TestLongerPaths(t *testing.T) {
	// desc := "Check requests to arbitrary paths with multiple directories"

	methods := []string{Get}
	paths := []string{
		"/a/tale/of/two/cities",
		"/it/was/the/best/of/times",
		"/it/was/the/worst/of/times",
	}
	routes := MakeRoutes(methods, paths, nil)
	requests := MakeRequests(methods, paths, paths, nil)

	test := NewTest(routes, requests)
	test.simple = true
	RunTest(t, test)
}

func TestShorterPaths(t *testing.T) {
	// desc := "Check requests to very short paths"

	methods := []string{Get}
	paths := []string{
		"",
		"/",
		// "//", // malformed, but the spec asks students to ignore leading/trailing /
		"/a",
		"/a1",
	}
	routes := MakeRoutes(methods, paths, nil)
	requests := MakeRequests(methods, paths, paths, nil)

	test := NewTest(routes, requests)
	test.simple = true
	RunTest(t, test)
}

// Test that slashes are not discriminating features at the start or end of a path
func TestPathSlashes(t *testing.T) {
	// desc := "Check that leading and trailing slashes in paths are ignored"

	methods := []string{Get}
	rtPaths := []string{
		"/path/",
		"/path/to/file/",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{
		"path",
		"/path",
		"path/",
		"/path/",
		"path/to/file",
		"/path/to/file",
		"path/to/file/",
		"/path/to/file/",
	}
	expRts := []string{
		"/path/",
		"/path/",
		"/path/",
		"/path/",
		"/path/to/file/",
		"/path/to/file/",
		"/path/to/file/",
		"/path/to/file/",
	}
	requests := MakeRequests(methods, rqPaths, expRts, nil)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test that case sensitivity is correctly handled in paths
func TestPathCaseSensitivity(t *testing.T) {
	// desc := "Check that paths are case-sensitive"

	methods := []string{Get}
	paths := []string{
		"/PATH",
		"/Path",
		"/pAth",
		"/paTH",
		"/PAth",
		"/PATh",
		"/PaTh",
	}
	expected := []string{NotFound} // If paths should not be case sensitive, change to Pass
	routes := MakeRoutes(methods, paths, nil)
	requests := MakeRequests(methods, paths, expected, nil)

	test := NewTest(routes, requests)
	test.simple = true
	RunTest(t, test)
}

// Test that paths are overwritten when added more than once
// Assuming for the time being this is intended behavior
func TestPathOverwriting(t *testing.T) {
	// desc := "Check that adding a route more than once updates its handler"

	methods := []string{Get}
	rtPaths := []string{
		"/examplepathA",
		"/examplepathB",
		"/examplepathC",
		"/examplepathA",
		"/examplepathB",
	}

	handlers := []http.HandlerFunc{OverwriteHandler, OverwriteHandler, nil}

	routes := MakeRoutes(methods, rtPaths, handlers)

	rqPaths := rtPaths[:3]
	expected := rqPaths
	requests := MakeRequests(methods, rqPaths, expected, nil)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test that intermediate paths do not resolve to ancestors or descendants
func TestIntermediatePaths(t *testing.T) {
	// desc := "Check unroutable paths that are prefixes of routable ones (and vice versa)"

	methods := []string{Get}
	rqPaths := []string{
		"/a",
		"/a/b",
		"/a/b/c",
		"/a/b/c/d",
		"/a/b/c/d/e",
		"/a/b/c/d/e/f",
		"/a/b/c/d/e/f/g",
		"/a/b/c/d/e/f/g/h",
		"/a/b/c/d/e/f/g/h/i",
	}
	n := len(rqPaths) - 1

	rtPaths := []string{rqPaths[0], rqPaths[n]}
	routes := MakeRoutes(methods, rtPaths, nil)

	expected := make([]string, len(rqPaths))
	for i := range expected {
		expected[i] = NotFound
	}
	expected[0] = rqPaths[0]
	expected[n] = rqPaths[n]

	requests := MakeRequests(methods, rqPaths, expected, nil)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test that the longest possible matching path is used
func TestLongestPath(t *testing.T) {
	// desc := "Check that router finds the longest possible matching path"

	methods := []string{Get}
	paths := []string{
		"/a",
		"/a/b",
		"/a/b/c",
		"/a/b/c/d",
		"/a/b/c/d/e",
		"/a/b/c/d/e/f",
		"/a/b/c/d/e/f/g",
		"/a/b/c/d/e/f/g/h",
		"/a/b/c/d/e/f/g/h/i",
	}

	routes := MakeRoutes(methods, paths, nil)
	requests := MakeRequests(methods, paths, paths, nil)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test that paths with repeated directory names resolve correctly.
// (i.e. no mappings that consider only lowest level directory)
func TestRepeatedPathNames(t *testing.T) {
	// desc := "Check that routes can have subdirectories with same name as parent"

	methods := []string{Get}
	paths := []string{
		"/a",
		"/a/a",
		"/a/a/a",
		"/a/a/a/a",
		"/a/a/a/a/a",
	}
	routes := MakeRoutes(methods, paths, nil)
	requests := MakeRequests(methods, paths, paths, nil)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

/*-----------------------------------------------------------------------------
 *                              Testing Captures
 -----------------------------------------------------------------------------*/

// Test that paths with captures can be resolved at all (ignore keys & values)
func TestCaptureResolves(t *testing.T) {
	// desc := "Check that routes with captures can be resolved at all, ignoring whether RawQuery is correct"

	methods := []string{Get}
	rtPaths := []string{"/:name"}
	rqPaths := []string{
		"alice",
		"bob",
		"eve",
	}

	handlers := []http.HandlerFunc{RouteHandler(rtPaths[0])}

	routes := MakeRoutes(methods, rtPaths, handlers)
	requests := MakeRequests(methods, rqPaths, rtPaths, nil)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test that basic capture functionality works, with correct keys & values
func TestBasicCapture(t *testing.T) {
	// desc := "Check that capture yields correct RawQuery"

	methods := []string{Get}
	rtPaths := []string{"/:name"}
	rqPaths := []string{
		"alice",
		"bob",
		"eve",
	}

	routes := MakeRoutes(methods, rtPaths, nil)

	expRts := rtPaths
	expRQs := []string{
		"name=alice",
		"name=bob",
		"name=eve",
	}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test that capture works for longer paths
func TestArbitraryPathCapture(t *testing.T) {
	// desc := "Check that capture works at an arbitrary position in an arbitrary path"

	methods := []string{Get}
	pre := []string{
		"",
		"/a/b/c/d/e/f/",
		"/a/b/c/d/e/f/g/h/i/j/k/l/",
	}
	post := []string{
		"/a/b/c/d/e/f/g/h/i/j/k/l/",
		"/g/h/i/j/k/l/",
		"",
	}
	vals := []string{
		"alice",
		// "bob",
		// "eve",
	}

	rtPaths := make([]string, len(pre))
	for i := range pre {
		rtPaths[i] = fmt.Sprintf("%s:name%s", pre[i], post[i])
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	// One request per name per capture location
	n := len(pre) * len(vals)
	rqPaths := make([]string, n)
	expRts := make([]string, n)
	expRQs := make([]string, n)
	for j, path := range rtPaths {
		for k, val := range vals {
			i := j*len(vals) + k
			rqPaths[i] = fmt.Sprintf("%s%s%s", pre[j], vals[k], post[j])
			expRts[i] = path
			expRQs[i] = fmt.Sprintf("name=%s", val)
		}
	}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test that several captures can appear in a single expression
func TestMultipleCaptures(t *testing.T) {
	// desc := "Check that several captures can appear in one route"

	methods := []string{Get}
	rtPaths := []string{
		"/person/:name/:age",
		"/add/:a/:b/:c/:d",
		"/sub/:e/:f/:g/:h/results",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	people := [][]string{
		{"Alice", "11"},
		{"Bob", "22"},
		{"Eve", "55"},
	}
	nums := [][]int{
		{316, 317, 318, 319},
		{0, 0, 1, 2},
		{3, 5, 8, 13},
	}

	n := len(people) + 2*len(nums)
	rqPaths := make([]string, n)
	expRts := make([]string, n)
	expRQs := make([]string, n)

	for i, p := range people {
		rqPaths[i] = fmt.Sprintf("/person/%s/%s", p[0], p[1])
		expRts[i] = rtPaths[0]
		expRQs[i] = fmt.Sprintf("age=%s&name=%s", p[1], p[0])
	}
	for j, n := range nums {
		i := len(people) + j
		rqPaths[i] = fmt.Sprintf("/add/%d/%d/%d/%d", n[0], n[1], n[2], n[3])
		expRts[i] = rtPaths[1]
		expRQs[i] = fmt.Sprintf("a=%d&b=%d&c=%d&d=%d", n[0], n[1], n[2], n[3])
	}
	for j, n := range nums {
		i := len(people) + len(nums) + j
		rqPaths[i] = fmt.Sprintf("/sub/%d/%d/%d/%d/results", n[0], n[1], n[2], n[3])
		expRts[i] = rtPaths[2]
		expRQs[i] = fmt.Sprintf("e=%d&f=%d&g=%d&h=%d", n[0], n[1], n[2], n[3])
	}

	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test capturing several values under a single key
func TestCaptureMultipleValues(t *testing.T) {
	// desc := "Check that multiple values can be captured under a single key"

	methods := []string{Get}
	rtPaths := []string{"/:color/:color"}
	rqPaths := []string{
		"red/blue",
		"orange/black",
		"cyan/magenta",
	}

	routes := MakeRoutes(methods, rtPaths, nil)

	expRts := rtPaths
	expRQs := make([]string, len(rqPaths))
	for i, p := range rqPaths {
		vs := strings.Split(p, "/")
		expRQs[i] = fmt.Sprintf("color=%s&color=%s", vs[0], vs[1])
	}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

func TestCaptureKeyOverwriting(t *testing.T) {
	// desc := "Check that keys are overwritten when adding several capturing routes"

	methods := []string{Get}
	captures := []string{
		":age",
		":password",
		":name",
	}

	rtPaths := make([]string, len(captures))
	copy(rtPaths, captures)
	for _, p := range captures {
		rtPaths = append(rtPaths, fmt.Sprintf("/user/%s/files", p))
	}

	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{
		"/alice",
		"/user/alice/files",
	}
	expRts := []string{
		"/:name",
		"/user/:name/files",
	}
	expRQs := []string{"name=alice"}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Precedence Tests (i.e. Trickier corner cases)

// Test behavior when a capturing and non-capturing route both apply.
func TestAmbiguousPaths1(t *testing.T) {
	// desc := "Check that non-capturing routes have precedence over capturing ones"

	methods := []string{Get}
	rtPaths := []string{
		"/path/to/:file",
		"/path/to/file",
		"/media/videos",
		"/media/:videos",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{
		"/path/to/readme.txt",
		"/path/to/file",
		"/media/videos",
		"/media/video316",
	}
	expRts := rtPaths
	expRQs := []string{
		"file=readme.txt",
		"",
		"",
		"videos=video316",
	}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test what happens when there are more ambiguous routes.
func TestAmbiguousPaths2(t *testing.T) {
	// desc := "Check non-capturing route precedence with ambiguous routes"

	methods := []string{Get}
	rtPaths := []string{
		"/animals/mammals/:species",
		"/animals/:type/images",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{
		// Standard captures
		"/animals/felines/images",
		"/animals/mammals/tigers",
		// Ambiguous capture
		"/animals/mammals/images",
		// No route exists
		"/animals/type/species",
	}
	expRts := []string{
		"/animals/:type/images",
		"/animals/mammals/:species",
		"/animals/mammals/:species",
		NotFound,
	}
	expRQs := []string{
		"type=felines",
		"species=tigers",
		"species=images",
		"",
	}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test what happens when there are extremely ambiguous routes.
func TestAmbiguousPaths3(t *testing.T) {
	// desc := "Check non-capturing route precedence with very ambiguous routes"

	methods := []string{Get}
	rtPaths := []string{
		"/:a/:b/:c/:d/:e",
		"/a/b/:c/:d/:e", // correct route
		"/a/:b/c/:d/e",
		"/:a/b/c/d/:e",
		"/:a/b/:c/d/:e",
		"/a/:b/c/:d/e",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{
		"/a/b/c/d/e",
	}
	expRts := []string{
		"/a/b/:c/:d/:e",
	}
	expRQs := []string{
		"c=c&d=d&e=e",
	}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Check for improper scoring schemes, as in bevin-grace, which may cause
// precedence to *appear* correct without actually being correct.
func TestAmbiguousPaths4(t *testing.T) {
	// desc := "Check non-capturing route precedence with longer ambiguous routes"
	methods := []string{Get}
	rtPaths := []string{
		"/:a/b/:c/:d/:e/:f/:g/:h/:i/:j", // correct route, but with a large number of captures
		"/:a/:b/c/d/e/f/g/h/i/j",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{
		"/a/b/c/d/e/f/g/h/i/j",
	}
	expRts := []string{
		"/:a/b/:c/:d/:e/:f/:g/:h/:i/:j",
	}
	expRQs := []string{
		"a=a&c=c&d=d&e=e&f=f&g=g&h=h&i=i&j=j",
	}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test that a resolver is not being greedy
func TestGreedyResolver(t *testing.T) {
	// desc := "Check that greedy resolution doesn't cause router to ignore valid routes"

	methods := []string{Get}
	rtPaths := []string{
		"/this/path/:will/work",
		"/this/path/willnot/",
		"/this/willnot",
		"/this/:will/work",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{
		"/this/path/willnot/work",
		"/this/willnot/work",
	}
	expRts := []string{
		"/this/path/:will/work",
		"/this/:will/work",
	}
	expRQs := []string{
		"will=willnot",
		"will=willnot",
	}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test what happens when there are very ambiguous routes.
func TestGreedyAmbiguousPaths1(t *testing.T) {
	// desc := "Check precedence and greedy resolution with ambiguous routes"

	methods := []string{Get}
	rtPaths := []string{
		"/:a/b/c/d/e",
		"/a/:b/c/d/e",
		"/a/b/:c/d/e", // correct
		"/a/b/c/:d/f",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{"/a/b/c/d/e"}
	expRts := []string{"/a/b/:c/d/e"}
	expRQs := []string{"c=c"}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Test what happens when there are extremely ambiguous routes.
func TestGreedyAmbiguousPaths2(t *testing.T) {
	// desc := "Check precedence and greedy resolution with very ambiguous routes"

	methods := []string{Get}
	rtPaths := []string{
		"/:a/:b/c/d/e",
		"/:a/b/c/d/:e", // correct
		"/:a/:b/:c/d/f",
		"/:a/:b/:c/:d/e",
		"/a/b/c/:d/f",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{"/a/b/c/d/e"}
	expRts := []string{"/:a/b/c/d/:e"}
	expRQs := []string{"a=a&e=e"}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

// Notice that capture keys are a property of the entire route, not of any
// single directory along the route. This test catches implementations that
// attempt to store capture keys "locally" associated with a specific directory,
// instead of nonlocally, associated with the entire route
func TestNonlocalCaptureOverwriting(t *testing.T) {
	// desc := "Check that capture keys are only overwritten when absolutely necessary"

	methods := []string{Get}
	rtPaths := []string{
		"/media/:dummy",
		"/media/:album/view",            // should not overwrite
		"/media/:photos/album/:album",   // should not overwrite
		"/media/:videos/:album/:videos", // should partially overwrite (?)
		"/media/:user",
	}
	routes := MakeRoutes(methods, rtPaths, nil)

	rqPaths := []string{
		"/media/turing",
		"/media/summer19/view",
		"/media/nofilter/album/fall19",
		"/media/slowmo/spring19/fullscreen",
	}
	expRts := []string{
		"/media/:user",
		"/media/:album/view",
		"/media/:photos/album/:album",
		"/media/:videos/:album/:videos",
	}
	expRQs := []string{
		"user=turing",
		"album=summer19",
		"photos=nofilter&album=fall19",
		"videos=slowmo&album=spring19&videos=fullscreen",
	}
	requests := MakeRequests(methods, rqPaths, expRts, expRQs)

	test := NewTest(routes, requests)
	RunTest(t, test)
}

/*-----------------------------------------------------------------------------
 *                           Performance
 -----------------------------------------------------------------------------*/

// benchmarkLookupMissNocapture fills a router with n non-capturing routes,
// and then times how long it takes for search misses to complete.
//func benchmarkLookupMissNocapture(n int, b *testing.B) {
//	router := http_router.NewRouter()
//	for i := 0; i < n; i++ {
//		path := RandomString(15)
//		router.AddRoute(Get, path, RouteAndQueryHandler(path))
//	}
//
//	request := NewRequest(Get, "searchmiss") // not len 15 so always misses
//	response := &SelfWriter{}
//
//	b.ResetTimer()
//	for i := 0; i < b.N; i++ {
//		router.ServeHTTP(response, request)
//	}
//}
//
//func benchmarkLookupMissNocaptureN(n int) func(*testing.B) {
//	return func(b *testing.B) {
//		benchmarkLookupMissNocapture(n, b)
//	}
//}
//
//func TestPerformanceMissNocapture(t *testing.T) {
//	L := &benchmark.Constant{
//		Min:   10,
//		Step:  10,
//		Max:   100000,
//		Fudge: 3.0, // Raise this if legit solutions fail timing tests
//	}
//	L.Test(t, benchmarkLookupMissNocaptureN)
//}
//
//// benchmarkLookupMissCapture fills a router with n capturing routes,
//// and then times how long it takes for search misses to complete.
//func benchmarkLookupMissCapture(n int, b *testing.B) {
//	router := http_router.NewRouter()
//	for i := 0; i < n; i++ {
//		rnd := RandomString(15)
//		path := fmt.Sprintf("/:%s/%s", rnd, rnd)
//		router.AddRoute(Get, path, RouteAndQueryHandler(path))
//	}
//
//	request := NewRequest(Get, "/searchmiss/pass") // 2nd comp. is not len 15 so always misses
//	response := &SelfWriter{}
//
//	b.ResetTimer()
//	for i := 0; i < b.N; i++ {
//		router.ServeHTTP(response, request)
//	}
//}
//
//func benchmarkLookupMissCaptureN(n int) func(*testing.B) {
//	return func(b *testing.B) {
//		benchmarkLookupMissCapture(n, b)
//	}
//}
//
//func TestPerformanceMissCapture(t *testing.T) {
//	L := &benchmark.Constant{
//		Min:   10,
//		Step:  10,
//		Max:   100000,
//		Fudge: 3.0, // Raise this if legit solutions fail timing tests
//	}
//	L.Test(t, benchmarkLookupMissCaptureN)
//}

/*-----------------------------------------------------------------------------
 *                       Notes & Scratchwork
 -----------------------------------------------------------------------------*/

// Not a real test. Prints final output after other tests are complete.
// func TestSummary(t *testing.T) {
// 	if outputEnabled {
// 		fmt.Printf(testStartMessage, testNo, "Test summary")
// 		fmt.Printf("   %d tests passed, %d tests failed\n", testsPassed, testsFailed)
// 		if testsFailed == 0 {
// 			fmt.Println(allTestsPassedMessage)
// 		}
// 	}
// }

// Example of a simple test without all of the testing framework.
// We could provide something like this to students to help them write their own

// Test that the simplest GET, POST, PUT, DELETE requests work in the best case
// func TestBasicMethods(t *testing.T) {
//   fmt.Println("Testing basic methods...")
//   methods := []string{GET, POST, PUT, DELETE}
//   for _, method := range methods {
//     w := new(SelfWriter)
//
//     request := NewRequest(method, "/examplepath")
//
//     router := NewRouter()
//     router.AddRoute(method, "/examplepath", PassHandler)
//     router.ServeHTTP(w, request)
//
//     if w.out != PASS {
//       t.Errorf("Failed to resolve basic %s request", method)
//     }
//   }
// }

// // A test that can be copy-pasted to make new tests
// func TestTemplate(t *testing.T) {
// 	routes := []Route{}
// 	requests := []Request{}
//
// 	err := RunTest(routes, requests)
// 	if err != nil {
// 		t.Errorf("Error message: \n%s\n", err)
// 	}
// }

/*-----------------------------------------------------------------------------
 *                    Ideas for further testing
 -----------------------------------------------------------------------------*/
/*

Ways to fail already being tested :
 Methods:
  - Methods are hard-coded / some required methods unsupported
  - Methods are case-sensitive
  - Not properly discriminating by method (GET/POST/...)
    - A route for method X works for some other method Y
 Paths (no capturing):
  - Added a route but it is not being used when it should. (all tests check this)
  - Using a route that was not added (e.g. hardcoding "/", "/index.html")
  - Paths can only be one-deep (long paths fail)
  - Paths are case-(in)sensitive (based on spec)
  - ? AddRoute does not overwrite an existing path when called w/ same params but different handler
  - Intermediate paths resolve correctly to an ancestor instead of giving 404
  - Shorter matching subpaths are returned instead of exact longest match
  - Can't handle paths with repeated directory names (e.g. /a/a/a/a)
Capturing:
  - Paths with capture resolve correctly but don't set params correctly
  - Captures give correct values but incorrect keys
  - Captures give correct keys but incorrect values
  - Basic single captures
  - Captures at the end of longer paths
  - Captures in the middle of longer paths
  - Paths with captures resolve literally (capture not implemented)
    - e.g. route for /path/:capture has /path/:capture resolve but not /path/example
  - Multiple captures - e.g. Can only capture once per path
  - Keys do not support multiple values
  - Behavior when a capture path and noncapture path both apply
  - Check for greedy resolvers
      Behavior when a capture path and non-capture path both apply,
       but trying to resolve the non-preferred path results in success
       while trying to resolve the preferred path results in failure
     - Note:  my own solution doesn't handle this yet

Ways to fail not already being tested
Possible ways for simple implementation to fail:
	- Check for basic properties of the request being preserved
		- Body
		- Relevant headers
  - ? Empty paths or directories (e.g. malformed paths like "////dir" )
    - I think we should not test for this and just leave behavior undefined.

Possible ways for capturing to fail:
  - ? Behavior when adding capture for path that already has a capture (overwrite key?)
  - ? case sensitivity of keys and values
	- ? Behavior when adding two capturing paths that have different keys?
		- explicitly overwrites
		- e.g. /path/to/:foo already exists, and /path/to/:bar is added
	- ? Behavior when adding a capture path that implicitly changes an earlier key?
		- eg. /path/to/:foo/bar already exists and /path/to/:bar/foo added

*/

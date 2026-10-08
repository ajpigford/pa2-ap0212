package benchmark

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

/******************************************************************************
 *                            Constants
 ******************************************************************************/

const failMessage = `

***** Benchmark too slow! *****
A %dx increase in the number of operations
resulted in a %.2fx increase to execution time!

(The maximum allowed ratio is %.2fx)

`

const resultsMessage = `Here are your complete benchmark results:

%s

`

/******************************************************************************
 *
 ******************************************************************************/

// A BenchFunc is a function that takes in an int M, the number of times some
// operation should be repeated, and then returns a func that runs a benchmark
// for M of some operation.
type BenchFunc func(int) func(*testing.B)

type BenchmarkResult struct {
	testing.BenchmarkResult     // the standard results provided by `testing`
	M                       int // the value of M being used for this benchmark
}

type BenchmarkResults []BenchmarkResult

func (rs BenchmarkResults) String() string {
	var b strings.Builder
	// golang already used N for number of trials, but we want to use it for "input size"
	//             |<-- 11  -->|<--  12  -->|<---       23       -->|<--   13  -->|
	b.WriteString("|     N     |   trials   |          time         |    ratio    |\n")
	b.WriteString("|-----------+------------+-----------------------+-------------|\n")
	for i, r := range rs {

		// Print out input size (we call this N in the table and M in the code)
		nstr := fmt.Sprintf("|  %7d  ", r.M)
		b.WriteString(nstr)

		// Print out # of trials (we call this )
		tstr := fmt.Sprintf("|  %8d  ", r.N)
		b.WriteString(tstr)

		// Print out time per operation:
		// Borrowing formatting code from golang's `package testing`:
		nsop := r.NsPerOp()
		ns := fmt.Sprintf("%10d ns/op", nsop)
		if r.N > 0 && nsop < 100 {
			// The format specifiers here make sure that
			// the ones digits line up for all three possible formats.
			if nsop < 10 {
				ns = fmt.Sprintf("%13.2f ns/op", float64(r.T.Nanoseconds())/float64(r.N))
			} else {
				ns = fmt.Sprintf("%12.1f ns/op", float64(r.T.Nanoseconds())/float64(r.N))
			}
		}
		nsstr := fmt.Sprintf("|  %19s  ", ns)
		b.WriteString(nsstr)

		// Print out the ratio for this operation.
		rstr := "|      -      "
		if i > 0 {
			prev := rs[i-1]
			ratio := float64(nsop) / float64(prev.NsPerOp())
			rstr = fmt.Sprintf("|  %8.2fx  ", ratio)
		}
		b.WriteString(rstr)

		// Finish the row
		b.WriteString("|\n")
	}
	return b.String()
}

/******************************************************************************
 *                            Linear
 ******************************************************************************/

// Linear describes a benchmark for which it is expected that the running time
// grows as a (continuous) linear function of the input size.
type Linear struct {
	Min     int              // the min value of M
	Step    int              // how many times larger each M will be than the previous n
	Max     int              // the max value of M
	Fudge   float64          // student ratio can be up to Fudge*Step before failing test
	Results BenchmarkResults // results of testing
}

// RatioLimit returns the maximum ratio a student may reach without failing.
func (L *Linear) RatioLimit() float64 {
	return float64(L.Step) * L.Fudge
}

// failMessage() generates a message indicating test failure.
func (L *Linear) failMessage(ratio float64) string {
	if L.Results == nil {
		return "Could not generate benchmark failure message - benchmark not yet run!"
	}

	return fmt.Sprintf(failMessage, L.Step, ratio, L.RatioLimit())
}

func (L *Linear) resultsMessage() string {
	return fmt.Sprintf(resultsMessage, L.Results)
}

// Benchmark running f M times, for every M specified by L. Store the results
// in L to be used later.
func (L *Linear) Test(t *testing.T, f BenchFunc) {

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	devnull, _ := os.Open("/dev/null")
	os.Stdout = devnull
	os.Stderr = devnull
	// Run a benchmark for function f, with an input size of m,
	// for every m in the range specified by L.
	results := []BenchmarkResult{}
	for m := L.Min; m <= L.Max; m *= L.Step {
		testRes := testing.Benchmark(f(m))
		results = append(results, BenchmarkResult{testRes, m})
	}
	L.Results = results
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	devnull.Close()

	// Check if any of the results had timing ratios outside of prescribed ranges
	for i := 1; i < len(results); i++ {
		curr := results[i]
		prev := results[i-1]

		// If runtime grows too quickly as a function of input size, fail test.
		// Consider a concrete example where we fix fudge=2, step=10:
		// fail if a 10x increase to input causes a >20x increase to runtime
		ratio := float64(curr.NsPerOp()) / float64(prev.NsPerOp())
		if ratio > L.RatioLimit() {
			t.Errorf(L.failMessage(ratio))
			t.Logf(L.resultsMessage())
			t.FailNow()
		}
	}
	t.Logf(L.resultsMessage())
}

/******************************************************************************
 *                            Constant
 ******************************************************************************/

// Constant describes a benchmark for which it is expected that the running time
// does not grow as a function of the input size.
// Strictly speaking, the running time is allowed to increase, but it must
// do so in a sublinear fashion.
// Care must be taken when selecting Step and Fudge - choosing Fudge >> Step, for
// example, will allow linear (or faster!) growth to pass this test.
type Constant struct {
	Min     int              // the min value of M
	Step    int              // how many times larger each M will be than the previous M
	Max     int              // the max value of M
	Fudge   float64          // student ratio can be up to Fudge before failing test
	Results BenchmarkResults // results of testing
}

// RatioLimit returns the maximum ratio a student may reach without failing.
func (C *Constant) RatioLimit() float64 {
	return C.Fudge
}

// failMessage() generates a message indicating test failure.
func (C *Constant) failMessage(ratio float64) string {
	if C.Results == nil {
		return "Could not generate benchmark failure message - benchmark not yet run!"
	}

	return fmt.Sprintf(failMessage, C.Step, ratio, C.RatioLimit())
}

func (C *Constant) resultsMessage() string {
	return fmt.Sprintf(resultsMessage, C.Results)
}

// Benchmark running f M times, for every M specified by L. Store the results
// in L to be used later.
func (C *Constant) Test(t *testing.T, f BenchFunc) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	devnull, _ := os.Open("/dev/null")
	os.Stdout = devnull
	os.Stderr = devnull

	// Run a benchmark for function f, with an input size of N,
	// for every N in the range specified by L.
	results := []BenchmarkResult{}
	for m := C.Min; m <= C.Max; m *= C.Step {
		testRes := testing.Benchmark(f(m))
		results = append(results, BenchmarkResult{testRes, m})
	}
	C.Results = results
	devnull.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	// Check if any of the results had timing ratios outside of prescribed ranges
	for i := 1; i < len(results); i++ {
		curr := results[i]
		prev := results[i-1]

		// If runtime grows too quickly as a function of input size, fail test
		// Consider a concrete example where we fix fudge=2 , step=10:
		// The test fails if a 10x increase to input size causes a >2x increase to runtime
		ratio := float64(curr.NsPerOp()) / float64(prev.NsPerOp())
		if ratio > C.RatioLimit() {
			t.Errorf(C.failMessage(ratio))
			t.Logf(C.resultsMessage())
			t.FailNow()
		}
	}
	t.Logf(C.resultsMessage())
}

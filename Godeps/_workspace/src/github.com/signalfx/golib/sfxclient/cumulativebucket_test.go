package sfxclient

import (
	"math/rand"
	"sync"
	"testing"

	"github.com/signalfx/golib/datapoint"
	. "github.com/smartystreets/goconvey/convey"
)

func dpNamed(name string, dps []*datapoint.Datapoint) *datapoint.Datapoint {
	for _, dp := range dps {
		if dp.Metric == name {
			return dp
		}
	}
	return nil
}

func TestCumulativeBucket(t *testing.T) {
	Convey("When bucket is setup", t, func() {
		cb := &CumulativeBucket{
			MetricName: "mname",
			Dimensions: map[string]string{"type": "dev"},
		}
		Convey("Empty bucket should be ok", func() {
			dps := cb.Datapoints()
			So(len(dps), ShouldEqual, 3)
			So(dpNamed("mname.sum", dps).Value.String(), ShouldEqual, "0")
		})
		Convey("No metric name should not send", func() {
			cb.MetricName = ""
			dps := cb.Datapoints()
			So(len(dps), ShouldEqual, 0)
		})
		Convey("adding a single point should make sense", func() {
			cb.Add(100)
			dps := cb.Datapoints()
			So(len(dps), ShouldEqual, 3)
			So(dpNamed("mname.sum", dps).Value.String(), ShouldEqual, "100")
			So(dpNamed("mname.count", dps).Value.String(), ShouldEqual, "1")
			So(dpNamed("mname.sumsquare", dps).Value.String(), ShouldEqual, "10000")
			Convey("and work with multiadd", func() {
				cb.MultiAdd(&Result{Count: 2, Sum: 9, SumOfSquares: 41})
				dps := cb.Datapoints()
				So(len(dps), ShouldEqual, 3)
				So(dpNamed("mname.sum", dps).Value.String(), ShouldEqual, "109")
				So(dpNamed("mname.count", dps).Value.String(), ShouldEqual, "3")
				So(dpNamed("mname.sumsquare", dps).Value.String(), ShouldEqual, "10041")
			})
			Convey("zero multiadd should do nothing", func() {
				cb.MultiAdd(&Result{})
				dps := cb.Datapoints()
				So(len(dps), ShouldEqual, 3)
				So(dpNamed("mname.sum", dps).Value.String(), ShouldEqual, "100")
				So(dpNamed("mname.count", dps).Value.String(), ShouldEqual, "1")
				So(dpNamed("mname.sumsquare", dps).Value.String(), ShouldEqual, "10000")
			})
		})
	})
}

func BenchmarkCumulativeBucket(b *testing.B) {
	cb := &CumulativeBucket{}
	r := rand.New(rand.NewSource(0))
	for i := 0; i < b.N; i++ {
		cb.Add(int64(r.Intn(1024)))
	}
}

func BenchmarkCumulativeBucket10(b *testing.B) {
	benchCB(b, 10)
}

func benchCB(b *testing.B, numGoroutine int) {
	cb := &CumulativeBucket{}
	w := sync.WaitGroup{}
	w.Add(numGoroutine)
	for g := 0; g < numGoroutine; g++ {
		go func(g int) {
			r := rand.New(rand.NewSource(0))
			for i := g; i < b.N; i += numGoroutine {
				cb.Add(int64(r.Intn(1024)))
			}
			w.Done()
		}(g)
	}
	w.Wait()
}

func BenchmarkCumulativeBucket100(b *testing.B) {
	benchCB(b, 100)
}

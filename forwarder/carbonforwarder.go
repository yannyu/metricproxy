package forwarder

import (
	"bytes"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/cep21/gohelpers/structdefaults"
	"github.com/cep21/gohelpers/workarounds"
	"github.com/signalfuse/signalfxproxy/config"
	"github.com/signalfuse/signalfxproxy/core"
	"github.com/signalfuse/signalfxproxy/protocoltypes"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type reconectingGraphiteCarbonConnection struct {
	*basicBufferedForwarder
	openConnection    net.Conn
	connectionAddress string
	connectionTimeout time.Duration
	connectionLock    sync.Mutex
}

// NewTcpGraphiteCarbonForwarer creates a new forwarder for sending points to carbon
func newTcpGraphiteCarbonForwarer(host string, port uint16, timeout time.Duration, bufferSize uint32, name string, drainingThreads uint32) (*reconectingGraphiteCarbonConnection, error) {
	connectionAddress := net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
	var d net.Dialer
	d.Deadline = time.Now().Add(timeout)
	conn, err := d.Dial("tcp", connectionAddress)
	if err != nil {
		return nil, err
	}
	ret := &reconectingGraphiteCarbonConnection{
		basicBufferedForwarder: newBasicBufferedForwarder(bufferSize, 100, name, drainingThreads),
		openConnection:         conn,
		connectionTimeout:      timeout,
		connectionAddress:      connectionAddress}
	ret.start(ret.drainDatapointChannel)
	return ret, nil
}

func (carbonConnection *reconectingGraphiteCarbonConnection) GetStats() []core.Datapoint {
	ret := []core.Datapoint{}
	return ret
}

var defaultCarbonConfig = &config.ForwardTo{
	TimeoutDuration: workarounds.GolangDoesnotAllowPointerToTimeLiteral(time.Second * 30),
	BufferSize:      workarounds.GolangDoesnotAllowPointerToUintLiteral(uint32(10000)),
	Port:            workarounds.GolangDoesnotAllowPointerToUint16Literal(2003),
	DrainingThreads: workarounds.GolangDoesnotAllowPointerToUintLiteral(uint32(1)),
	Name:            workarounds.GolangDoesnotAllowPointerToStringLiteral("carbonforwarder"),
	MaxDrainSize:    workarounds.GolangDoesnotAllowPointerToUintLiteral(uint32(1000)),
}

// TcpGraphiteCarbonForwarerLoader loads a carbon forwarder
func TcpGraphiteCarbonForwarerLoader(forwardTo *config.ForwardTo) (core.StatKeepingStreamingAPI, error) {
	structdefaults.FillDefaultFrom(forwardTo, defaultCarbonConfig)
	return newTcpGraphiteCarbonForwarer(*forwardTo.Host, *forwardTo.Port, *forwardTo.TimeoutDuration, *forwardTo.BufferSize, *forwardTo.Name, *forwardTo.DrainingThreads)
}

func (carbonConnection *reconectingGraphiteCarbonConnection) createClientIfNeeded() error {
	var err error
	if carbonConnection.openConnection == nil {
		carbonConnection.openConnection, err = net.Dial("tcp", carbonConnection.connectionAddress)
	}
	return err
}

func (carbonConnection *reconectingGraphiteCarbonConnection) datapointToGraphite(datapoint core.Datapoint) string {
	ret := []string{}
	// Note: Key is unused.  It's ambiguous how to add this.  Also, the dimensions aren't exactly
	//       ordered ....
	keys := []string{}
	dims := datapoint.Dimensions()
	for k := range dims {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ret = append(ret, dims[k])
	}
	ret = append(ret, datapoint.Metric())
	return strings.Join(ret, ".")
}

func (carbonConnection *reconectingGraphiteCarbonConnection) drainDatapointChannel(datapoints []core.Datapoint) error {
	if err := carbonConnection.createClientIfNeeded(); err != nil {
		return err
	}
	err := carbonConnection.openConnection.SetDeadline(time.Now().Add(carbonConnection.connectionTimeout))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, datapoint := range datapoints {
		carbonReadyDatapoint, ok := datapoint.(protocoltypes.CarbonReady)
		if ok {
			fmt.Fprintf(&buf, "%s\n", carbonReadyDatapoint.ToCarbonLine())
		} else {
			fmt.Fprintf(&buf, "%s %s %d\n", carbonConnection.datapointToGraphite(datapoint),
				datapoint.Value().WireValue(),
				datapoint.Timestamp().UnixNano()/time.Second.Nanoseconds())
		}
	}
	log.WithField("buf", buf).Debug("Will write to graphite")
	_, err = buf.WriteTo(carbonConnection.openConnection)
	if err != nil {
		return err
	}

	return nil
}

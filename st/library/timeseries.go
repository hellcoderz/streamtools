package library

import (
	"errors"
	"github.com/mjibson/go-dsp/fft"            // fft
	"github.com/nytlabs/gojee"                 // jee
	"github.com/nytlabs/streamtools/st/blocks" // blocks
	"github.com/nytlabs/streamtools/st/util"   // util
	"time"
)

// specify those channels we're going to use to communicate with streamtools
type Timeseries struct {
	blocks.Block
	queryrule  chan chan interface{}
	querystate chan chan interface{}
	queryfft   chan chan interface{}
	inrule     chan interface{}
	inpoll     chan interface{}
	in         chan interface{}
	out        chan interface{}
	quit       chan interface{}
}

type tsDataPoint struct {
	Timestamp float64
	Value     float64
}

type tsData struct {
	Values []tsDataPoint
}

// we need to build a simple factory so that streamtools can make new blocks of this kind
func NewTimeseries() blocks.BlockInterface {
	return &Timeseries{}
}

// Setup is called once before running the block. We build up the channels and specify what kind of block this is.
func (b *Timeseries) Setup() {
	b.Kind = "Timeseries"
	b.Desc = "stores an array of values for a specified Path along with timestamps"
	b.in = b.InRoute("in")
	b.inrule = b.InRoute("rule")
	b.queryrule = b.QueryRoute("rule")
	b.querystate = b.QueryRoute("timeseries")
	b.queryfft = b.QueryRoute("fft")
	b.inpoll = b.InRoute("poll")
	b.quit = b.Quit()
	b.out = b.Broadcast()
}

// Run is the block's main loop. Here we listen on the different channels we set up.
func (b *Timeseries) Run() {

	var err error
	//var path, lagStr string
	var path string
	var tree *jee.TokenTree
	//var lag time.Duration
	var data tsData
	var numSamples float64

	// defaults
	numSamples = 1

	for {
		select {
		case ruleI := <-b.inrule:
			// set a parameter of the block
			rule, ok := ruleI.(map[string]interface{})
			if !ok {
				b.Error(errors.New("could not assert rule to map"))
			}
			path, err = util.ParseString(rule, "Path")
			if err != nil {
				b.Error(err)
				continue
			}
			tree, err = util.BuildTokenTree(path)
			if err != nil {
				b.Error(err)
				continue
			}
			/*
				lagStr, err = util.ParseString(rule, "Lag")
				if err != nil {
					b.Error(err)
					continue
				}
				lag, err = time.ParseDuration(lagStr)
				if err != nil {
					b.Error(err)
					continue
				}
			*/
			numSamples, err = util.ParseFloat(rule, "NumSamples")
			if err != nil {
				b.Error(err)
				continue
			}
			data = tsData{
				Values: make([]tsDataPoint, int(numSamples)),
			}

		case <-b.quit:
			// quit * time.Second the block
			return
		case msg := <-b.in:
			if tree == nil {
				continue
			}
			if data.Values == nil {
				continue
			}
			// deal with inbound data
			v, err := jee.Eval(tree, msg)
			if err != nil {
				b.Error(err)
				continue
			}
			var val float64
			switch v := v.(type) {
			case float32:
				val = float64(v)
			case int:
				val = float64(v)
			case float64:
				val = v
			}

			//t := float64(time.Now().Add(-lag).Unix())
			t := float64(time.Now().Unix())

			d := tsDataPoint{
				Timestamp: t,
				Value:     val,
			}
			data.Values = append(data.Values[1:], d)
		case respChan := <-b.queryrule:
			// deal with a query request
			respChan <- map[string]interface{}{
				//"Window":     lagStr,
				"Path":       path,
				"NumSamples": numSamples,
			}
		case respChan := <-b.querystate:
			out := map[string]interface{}{
				"timeseries": data,
			}
			respChan <- out
		case respChan := <-b.queryfft:
			x := make([]float64, len(data.Values))
			for i, d := range data.Values {
				x[i] = d.Value
			}
			X := fft.FFTReal(x)

			Xout := make([][]float64, len(X))
			for i, Xi := range X {
				Xout[i] = make([]float64, 2)
				Xout[i][0] = real(Xi)
				Xout[i][1] = imag(Xi)
			}
			out := map[string]interface{}{
				"fft": Xout,
			}
			respChan <- out
		case <-b.inpoll:
			outArray := make([]interface{}, len(data.Values))
			for i, d := range data.Values {
				di := map[string]interface{}{
					"timestamp": d.Timestamp,
					"value":     d.Value,
				}
				outArray[i] = di
			}
			out := map[string]interface{}{
				"timeseries": outArray,
			}
			b.out <- out
		}
	}
}

package main

import (
	"encoding/xml"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"time"
)

type TPX struct {
	Speed      string `xml:"Speed,omitempty"`
	RunCadence string `xml:"RunCadence,omitempty"`
}

type Trackpoint struct {
	Time     time.Time
	Position struct {
		LatitudeDegrees  string `xml:",omitempty"`
		LongitudeDegrees string `xml:",omitempty"`
	} `xml:",omitempty"`
	AltitudeMeters string `xml:",omitempty"`
	DistanceMeters string `xml:",omitempty"`

	HeartRateBpm struct {
		Value int `xml:",omitempty"`
	} `xml:",omitempty"`

	Extensions struct {
		TPX TPX
	} `xml:",omitempty"`
}

// when reading the xml, namespacing does not work ... so we read in w/o
// namespace information but marshal back with namespace info
func (t TPX) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	x := struct {
		XMLName    xml.Name `xml:"ns3:TPX"`
		RunCadence string   `xml:"ns3:RunCadence,omitempty"`
		Speed      string   `xml:"ns3:Speed,omitempty"`
	}{Speed: t.Speed, RunCadence: t.RunCadence}

	return e.Encode(x)
}

type TCX struct {
	XMLName        xml.Name `xml:TrainingCenterDatabase`
	SchemaLocation string   `xml:"xsi:schemaLocation,attr"`
	Ns5            string   `xml:"xmlns:ns5,attr"`
	Ns4            string   `xml:"xmlns:ns4,attr"`
	Ns3            string   `xml:"xmlns:ns3,attr"`
	Ns2            string   `xml:"xmlns:ns2,attr"`
	Ns             string   `xml:"xmlns,attr"`
	NsXsi          string   `xml:"xmlns:xsi,attr"`
	Activities     struct {
		Activity struct {
			Sport string `xml:"Sport,attr"`
			Id    string
			Lap   []struct {
				StartTime        time.Time `xml:"StartTime,attr"`
				TotalTimeSeconds string    `xml:",omitempty"`
				DistanceMeters   string    `xml:",omitempty"`
				MaximumSpeed     string    `xml:",omitempty"`
				Calories         string    `xml:",omitempty"`

				AverageHeartRateBpm struct {
					Value int `xml:",omitempty"`
				} `xml:",omitempty"`

				MaximumHeartRateBpm struct {
					Value int `xml:",omitempty"`
				} `xml:",omitempty"`

				Intensity     string `xml:",omitempty"`
				TriggerMethod string `xml:",omitempty"`
				Track         struct {
					Trackpoint []Trackpoint
				} `xml:",omitempty"`
			}
		}
	}
}

func trackpointIterator(tcx TCX) *chan Trackpoint {
	c := make(chan Trackpoint)

	go func() {
		for _, lap := range tcx.Activities.Activity.Lap {
			for _, trackpoint := range lap.Track.Trackpoint {
				c <- trackpoint
			}
		}
		close(c)
	}()

	return &c
}

func replaceTrackpoints(tcx TCX, trackpoints []Trackpoint) TCX {
	lapCnt := 0
	latestTrackpoint := trackpoints[len(trackpoints)-1].Time
	laps := tcx.Activities.Activity.Lap

	getNextLapStartTime := func() time.Time {
		if lapCnt == len(laps)-1 {
			return latestTrackpoint.Add(time.Second)
		}

		return laps[lapCnt+1].StartTime
	}

	nextLapStartTime := getNextLapStartTime()
	newTrackpoints := []Trackpoint{}

	type BpmInfo struct {
		cnt   int
		max   int
		total int
	}

	bpmInfo := BpmInfo{}

	for _, trackpoint := range trackpoints {
		if trackpoint.Time.After(nextLapStartTime) || trackpoint.Time.Equal(nextLapStartTime) {
			tcx.Activities.Activity.Lap[lapCnt].Track.Trackpoint = newTrackpoints
			if bpmInfo.cnt > 0 {
				tcx.Activities.Activity.Lap[lapCnt].AverageHeartRateBpm.Value = bpmInfo.total / bpmInfo.cnt
				tcx.Activities.Activity.Lap[lapCnt].MaximumHeartRateBpm.Value = bpmInfo.max
			}

			bpmInfo = BpmInfo{}
			newTrackpoints = []Trackpoint{}
			lapCnt++
			nextLapStartTime = getNextLapStartTime()
			continue
		}

		newTrackpoints = append(newTrackpoints, trackpoint)
		if bpm := trackpoint.HeartRateBpm.Value; bpm != 0 {
			bpmInfo.cnt++
			if bpmInfo.max < bpm {
				bpmInfo.max = bpm
			}
			bpmInfo.total += bpm
		}
	}
	tcx.Activities.Activity.Lap[lapCnt].Track.Trackpoint = newTrackpoints

	return tcx
}

func mergeTcx(master TCX, bpm TCX) (TCX, error) {
	trackpoints := []Trackpoint{}

	masterIter := trackpointIterator(master)
	bpmIter := trackpointIterator(bpm)

	masterTp, moreMasterTp := <-*masterIter
	bpmTp, moreBpmTp := <-*bpmIter

loop:
	for {
		switch {
		case !moreMasterTp && !moreBpmTp:
			break loop

		case (moreMasterTp && !moreBpmTp) || (moreMasterTp && masterTp.Time.Before(bpmTp.Time)):
			trackpoints = append(trackpoints, masterTp)
			masterTp, moreMasterTp = <-*masterIter

		case moreBpmTp && !moreMasterTp || (moreMasterTp && masterTp.Time.After(bpmTp.Time)):
			// we only want the heart beat info and the time stamp
			trackpoints = append(trackpoints, Trackpoint{HeartRateBpm: bpmTp.HeartRateBpm, Time: bpmTp.Time, Position: trackpoints[len(trackpoints)-1].Position})
			bpmTp, moreBpmTp = <-*bpmIter

		case masterTp.Time == bpmTp.Time:
			masterTp.HeartRateBpm = bpmTp.HeartRateBpm
			trackpoints = append(trackpoints, masterTp)
			masterTp, moreMasterTp = <-*masterIter
			bpmTp, moreBpmTp = <-*bpmIter
		}
	}

	return replaceTrackpoints(master, trackpoints), nil
}

func main() {
	masterFileName := flag.String("master-tcx", "", "file name of the tcx that should be enriched with heartbeat data")
	bpmFileName := flag.String("bpm-tcx", "", "file name of the tcx that contains bpm information")
	flag.Parse()

	if *masterFileName == "" || *bpmFileName == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	masterContent, err := ioutil.ReadFile(*masterFileName)
	if err != nil {
		log.Fatalf("error reading file %s: %s\n", *masterFileName, err)
	}

	bpmContent, err := ioutil.ReadFile(*bpmFileName)
	if err != nil {
		log.Fatalf("error reading file %s: %s\n", *bpmFileName, err)
	}

	var masterTcx TCX
	err = xml.Unmarshal([]byte(masterContent), &masterTcx)

	var bpmTcx TCX
	err = xml.Unmarshal([]byte(bpmContent), &bpmTcx)

	mergedTcx, err := mergeTcx(masterTcx, bpmTcx)
	if err != nil {
		log.Fatalf("error merging tcx files: %s", err)
	}

	output, err := xml.MarshalIndent(mergedTcx, "", "  ")
	if err != nil {
		log.Fatalf("error: %v\n", err)
	}
	os.Stdout.Write(output)
}

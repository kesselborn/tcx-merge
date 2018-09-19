package main

import (
	"encoding/xml"
	"flag"
	"io/ioutil"
	"log"
	"os"
)

type TPX struct {
	Speed      string `xml:"Speed,omitempty"`
	RunCadence string `xml:"RunCadence,omitempty"`
}

type Trackpoint struct {
	Time     string
	Position struct {
		LatitudeDegrees  string `xml:",omitempty"`
		LongitudeDegrees string `xml:",omitempty"`
	} `xml:",omitempty"`
	AltitudeMeters string `xml:",omitempty"`
	DistanceMeters string `xml:",omitempty"`
	HeartRateBpm   struct {
		Value string `xml:",omitempty"`
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
				StartTime           string `xml:"StartTime,attr"`
				TotalTimeSeconds    string `xml:",omitempty"`
				DistanceMeters      string `xml:",omitempty"`
				MaximumSpeed        string `xml:",omitempty"`
				Calories            string `xml:",omitempty"`
				AverageHeartRateBpm struct {
					Value string `xml:",omitempty"`
				} `xml:",omitempty"`
				MaximumHeartRateBpm struct {
					Value string `xml:",omitempty"`
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

func main() {
	masterFileName := flag.String("master-tcx", "", "file name of the tcx that should be enriched with heartbeat data")
	flag.Parse()

	if *masterFileName == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	masterContent, err := ioutil.ReadFile(*masterFileName)

	if err != nil {
		log.Fatalf("error reading file %s: %s\n", *masterFileName, err)
	}

	var tcx TCX
	err = xml.Unmarshal([]byte(masterContent), &tcx)

	output, err := xml.MarshalIndent(tcx, "", "  ")
	if err != nil {
		log.Fatalf("error: %v\n", err)
	}

	os.Stdout.Write(output)
}

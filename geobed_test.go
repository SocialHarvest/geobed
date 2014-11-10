package geobed

import (
	. "gopkg.in/check.v1"
	"testing"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type GeobedSuite struct {
	testLocations []map[string]string
}

var _ = Suite(&GeobedSuite{})

func (s *GeobedSuite) SetUpSuite(c *C) {
	s.testLocations = append(s.testLocations, map[string]string{"query": "New York, NY", "city": "New York", "country": "US", "region": "NY"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "New York", "city": "New York", "country": "US", "region": "NY"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "SFO", "city": "San Francisco", "country": "US", "region": "CA"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "Austin Tx", "city": "Austin", "country": "US", "region": "TX"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "tx austin", "city": "Austin", "country": "US", "region": "TX"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "london", "city": "London", "country": "GB", "region": "ENG"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "Paris", "city": "Paris", "country": "FR", "region": "A8"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "Paris, TX", "city": "Paris", "country": "US", "region": "TX"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "New Paris", "city": "New Paris", "country": "US", "region": "OH"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "New Paris, IN", "city": "New Paris", "country": "US", "region": "IN"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "ਪੈਰਿਸ", "city": "Paris", "country": "FR", "region": "A8"})
}

func (s *GeobedSuite) TestNewGeobed(c *C) {
	g := NewGeobed()
	c.Assert(len(g.c), Not(Equals), 0)
	c.Assert(len(g.co), Not(Equals), 0)
}

func (s *GeobedSuite) TestGeocode(c *C) {
	g := NewGeobed()
	for _, v := range s.testLocations {
		r := g.Geocode(v["query"])
		c.Assert(r.City, Equals, v["city"])
		c.Assert(r.Country, Equals, v["country"])
		c.Assert(r.Region, Equals, v["region"])
	}

	r := g.Geocode("")
	c.Assert(r.City, Equals, "")

	r = g.Geocode(" ")
	c.Assert(r.Population, Equals, 0)
}

func (s *GeobedSuite) TestReverseGeocode(c *C) {
	g := NewGeobed()

	r := g.ReverseGeocode(30.26715, -97.74306)
	c.Assert(r.City, Equals, "Austin")
	c.Assert(r.Region, Equals, "TX")
	c.Assert(r.Country, Equals, "US")

	r = g.ReverseGeocode(37.44651, -122.15322)
	c.Assert(r.City, Equals, "Palo Alto")
	c.Assert(r.Region, Equals, "CA")
	c.Assert(r.Country, Equals, "US")

	r = g.ReverseGeocode(37, -122)
	c.Assert(r.City, Equals, "Santa Cruz")

	r = g.ReverseGeocode(37.44, -122.15)
	c.Assert(r.City, Equals, "Stanford")
}

// 2285549904 ns/op
// 2393945317 ns/op
// 2214503806 ns/op
// 2265304148 ns/op
// 2186608767 ns/op
// This has been scoring around 2 - 2.3 seconds on my MacbookPro Retina with 8GB of RAM (before concurrency was added)
func BenchmarkReverseGeocode(b *testing.B) {
	g := NewGeobed()

	for n := 0; n < b.N; n++ {
		g.ReverseGeocode(37.44651, -122.15322)
	}
}

// 3034545386 ns/op
// 3240491478 ns/op
// 5304891006 ns/op
// 3283455985 ns/op
// 3176123247 ns/op
// This has been scoring around 3 - 5.3 seconds on my MacbookPro Retina with 8GB of RAM (before concurrency was added)
func BenchmarkGeocode(b *testing.B) {
	g := NewGeobed()

	for n := 0; n < b.N; n++ {
		g.Geocode("New York")
	}
}

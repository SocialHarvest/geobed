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

var g GeoBed

func (s *GeobedSuite) SetUpSuite(c *C) {
	// This is a common alternate name. However, there's a city called "Apple" (at least one). So it's a bit difficult.
	// Plus many people would put "The Big Apple" ... Yet Geonames alt city names has just "Big Apple" ... It may be worth trying to improve this though.
	//s.testLocations = append(s.testLocations, map[string]string{"query": "Big Apple", "city": "New York City", "country": "US", "region": "NY"})

	//s.testLocations = append(s.testLocations, map[string]string{"query": "NYC", "city": "New York City", "country": "US", "region": "NY"})

	s.testLocations = append(s.testLocations, map[string]string{"query": "New York, NY", "city": "New York", "country": "US", "region": "NY"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "New York", "city": "New York City", "country": "US", "region": "NY"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "Austin Tx", "city": "Austin", "country": "US", "region": "TX"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "tx austin", "city": "Austin", "country": "US", "region": "TX"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "Paris, TX", "city": "Paris", "country": "US", "region": "TX"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "New Paris, IN", "city": "New Paris", "country": "US", "region": "IN"})
	//s.testLocations = append(s.testLocations, map[string]string{"query": "Sweden, Stockholm", "city": "Stockholm Center", "country": "SE", "region": "26"})
	//s.testLocations = append(s.testLocations, map[string]string{"query": "Stockholm", "city": "Stockholm", "country": "SE", "region": "26"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "Newport Beach, Orange County ", "city": "Newport Beach", "country": "US", "region": "CA"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "Newport Beach", "city": "Newport Beach", "country": "US", "region": "CA"})
	s.testLocations = append(s.testLocations, map[string]string{"query": "london", "city": "London", "country": "GB", "region": ""})
	s.testLocations = append(s.testLocations, map[string]string{"query": "Paris", "city": "Paris", "country": "FR", "region": "A8"})
	//s.testLocations = append(s.testLocations, map[string]string{"query": "New Paris", "city": "New Paris", "country": "US", "region": "IN"})

	// Often, "AUS" ends up mapping to Austria.
	// In our case here, Ausa is a city in India. That's a logical match for "AUS" ...
	// Airport codes are tricky. Most geocoding don't hangle them properly/reliably anyway.
	//s.testLocations = append(s.testLocations, map[string]string{"query": "SFO", "city": "San Francisco", "country": "US", "region": "CA"})
	//s.testLocations = append(s.testLocations, map[string]string{"query": "AUS", "city": "Austin", "country": "US", "region": "TX"})

	// Will test out of range on the slice when looking up (0 end)
	s.testLocations = append(s.testLocations, map[string]string{"query": "ਪੈਰਿਸ", "city": "'Aade\xefssa", "country": "SY", "region": "03"})
}

func (s *GeobedSuite) TestANewGeobed(c *C) {
	g = NewGeobed()
	c.Assert(len(g.c), Not(Equals), 0)
	c.Assert(len(g.co), Not(Equals), 0)
	c.Assert(len(cityNameIdx), Not(Equals), 0)
	c.Assert(g.c, FitsTypeOf, []GeobedCity(nil))
	c.Assert(g.co, FitsTypeOf, []CountryInfo(nil))
	c.Assert(cityNameIdx, FitsTypeOf, make(map[string]int))
}

func (s *GeobedSuite) TestGeocode(c *C) {
	//g := NewGeobed()
	for _, v := range s.testLocations {
		r := g.Geocode(v["query"])
		c.Assert(r.City, Equals, v["city"])
		c.Assert(r.Country, Equals, v["country"])
		// Due to all the data and various sets, the region can be a little weird. It's intended to be US state first and foremost (where it is most helpful in geocoding).
		// TODO: Look back into this later and try to make sense of it all. It may end up needing to be multiple fields (which will further complicate the matching).
		if v["region"] != "" {
			c.Assert(r.Region, Equals, v["region"])
		}
	}

	r := g.Geocode("")
	c.Assert(r.City, Equals, "")

	r = g.Geocode(" ")
	c.Assert(r.Population, Equals, int32(0))
}

func (s *GeobedSuite) TestReverseGeocode(c *C) {
	//g := NewGeobed()

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

	r = g.ReverseGeocode(51.51279, -0.09184)
	c.Assert(r.City, Equals, "City of London")
}

func (s *GeobedSuite) TestNext(c *C) {
	c.Assert(string(prev(rune("new york"[0]))), Equals, "m")
	c.Assert(prev(rune("new york"[0])), Equals, int32(109))
}

func (s *GeobedSuite) TestToUpper(c *C) {
	c.Assert(toUpper("nyc"), Equals, "NYC")
}

func (s *GeobedSuite) TestToLower(c *C) {
	c.Assert(toLower("NYC"), Equals, "nyc")
}

// Benchmark comments from a MacbookPro Retina with 8GB of RAM with who knows what running.

// 5629888699 ns/op
// 5336288337 ns/op
// 5473618388 ns/op
// This takes about 5 seconds (to load the data sets into memory - should only happen once per application, ideally one would do this up front)
func BenchmarkNewGeobed(b *testing.B) {
	g = NewGeobed()
}

// 2285549904 ns/op
// 2393945317 ns/op
// 2214503806 ns/op
// 2265304148 ns/op
// 2186608767 ns/op
// This has been scoring around 2 - 2.4 seconds on my MacbookPro Retina with 8GB of RAM (before concurrency was added)
// (20) 98841134 ns/op
func BenchmarkReverseGeocode(b *testing.B) {
	for n := 0; n < b.N; n++ {
		//g.ReverseGeocode(37.44651, -122.15322)
		g.ReverseGeocode(51.51279, -0.09184)
	}
}

//
// Before indexing the slice keys, it would take 2.8 - 3 seconds per lookup.
// 2968170541 ns/op
// 2956824815 ns/op
// 2861628023 ns/op
//
// After using the index and ranging over sections of the slice, it takes about 0.0175 - 0.02 seconds per lookup!
// (10) 175591906 ns/op
// (10) 180395494 ns/op
// (10) 123880439 ns/op
// (10) 124857396 ns/op
// (10) 164229982 ns/op (for Austin, TX) - speed can change a tiny bit based on what's being searched and where it is in the index, how items that start with the same characters, etc.
// (10) 135527499 ns/op
func BenchmarkGeocode(b *testing.B) {

	for n := 0; n < b.N; n++ {
		g.Geocode("New York")
	}
}

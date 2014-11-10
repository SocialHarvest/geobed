package geobed

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/gob"
	geohash "github.com/TomiHiltunen/geohash-golang"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// There are over 2.4 million cities in the world. The Geonames data set only contains 143,270 and the MaxMind set contains 567,382 and 3,173,959 in the other MaxMind set.
// Obviously there's a lot of overlap and the worldcitiespop.txt from MaxMind contains a lot of dupes, though it by far the most comprehensive in terms of city - lat/lng.
// It may not be possible to have information for all cities, but many of the cities are also fairly remote and likely don't have internet access anyway.
// The Geonames data is preferred because it contains additional information such as elevation, population, and more. Population is good particuarly nice because a sense for
// the city size can be understood by applications. So showing all major cities is pretty easy. Though the primary goal of this package is to geocode, the additional information
// is bonus. So after checking the Geonames set, the geocoding functions will then look at MaxMind's.
// Maybe in the future this package will even use the Geonames premium data and have functions to look up nearest airports, etc.
// I would simply use just Geonames data, but there's so many more cities in the MaxMind set despite the lack of additional details.
//
// http://download.geonames.org/export/dump/cities1000.zip
// http://geolite.maxmind.com/download/geoip/database/GeoLiteCity_CSV/GeoLiteCity-latest.zip
// http://download.maxmind.com/download/worldcities/worldcitiespop.txt.gz

// A list of data sources.
var dataSetFiles = []map[string]string{
	{"url": "http://download.geonames.org/export/dump/cities1000.zip", "path": "./geobed-data/cities1000.zip", "id": "geonamesCities1000"},
	{"url": "http://download.geonames.org/export/dump/countryInfo.txt", "path": "./geobed-data/countryInfo.txt", "id": "geonamesCountryInfo"},
	{"url": "http://download.maxmind.com/download/worldcities/worldcitiespop.txt.gz", "path": "./geobed-data/worldcitiespop.txt.gz", "id": "maxmindWorldCities"},
	//{"url": "http://geolite.maxmind.com/download/geoip/database/GeoLiteCity_CSV/GeoLiteCity-latest.zip", "path": "./geobed-data/GeoLiteCity-latest.zip", "id": "maxmindLiteCity"},
}

// A handy map of US state codes to full names.
var UsSateCodes = map[string]string{
	"AL": "Alabama",
	"AK": "Alaska",
	"AZ": "Arizona",
	"AR": "Arkansas",
	"CA": "California",
	"CO": "Colorado",
	"CT": "Connecticut",
	"DE": "Delaware",
	"FL": "Florida",
	"GA": "Georgia",
	"HI": "Hawaii",
	"ID": "Idaho",
	"IL": "Illinois",
	"IN": "Indiana",
	"IA": "Iowa",
	"KS": "Kansas",
	"KY": "Kentucky",
	"LA": "Louisiana",
	"ME": "Maine",
	"MD": "Maryland",
	"MA": "Massachusetts",
	"MI": "Michigan",
	"MN": "Minnesota",
	"MS": "Mississippi",
	"MO": "Missouri",
	"MT": "Montana",
	"NE": "Nebraska",
	"NV": "Nevada",
	"NH": "New Hampshire",
	"NJ": "New Jersey",
	"NM": "New Mexico",
	"NY": "New York",
	"NC": "North Carolina",
	"ND": "North Dakota",
	"OH": "Ohio",
	"OK": "Oklahoma",
	"OR": "Oregon",
	"PA": "Pennsylvania",
	"RI": "Rhode Island",
	"SC": "South Carolina",
	"SD": "South Dakota",
	"TN": "Tennessee",
	"TX": "Texas",
	"UT": "Utah",
	"VT": "Vermont",
	"VA": "Virginia",
	"WA": "Washington",
	"WV": "West Virginia",
	"WI": "Wisconsin",
	"WY": "Wyoming",
	// Territories
	"AS": "American Samoa",
	"DC": "District of Columbia",
	"FM": "Federated States of Micronesia",
	"GU": "Guam",
	"MH": "Marshall Islands",
	"MP": "Northern Mariana Islands",
	"PW": "Palau",
	"PR": "Puerto Rico",
	"VI": "Virgin Islands",
	// Armed Forces (AE includes Europe, Africa, Canada, and the Middle East)
	"AA": "Armed Forces Americas",
	"AE": "Armed Forces Europe",
	"AP": "Armed Forces Pacific",
}

// Contains all of the city and country data. Cities are split into buckets by country to increase lookup speed when the country is known.
type GeoBed struct {
	c   []GeobedCity
	gc  []GeonamesCity
	mc  []MaxMindCity
	mcl []MaxMindCityLite
	co  []CountryInfo
}

// A combined city struct (the various data sets have different fields, this combines what's available and keeps things smaller)
type GeobedCity struct {
	City       string
	CityAlt    string
	Country    string
	Region     string
	Population int
	Latitude   float64
	Longitude  float64
	Geohash    string
}

// Geonames City (with population of 1,000 or greater)
type GeonamesCity struct {
	GeonameId        int
	Name             string
	AsciiName        string
	AlternateNames   string
	Latitude         float64
	Longitude        float64
	FeatureClass     string
	FeatureCode      string
	CountryCode      string
	CC2              string
	Admin1Code       string
	Admin2Code       string
	Admin3Code       string
	Admin4Code       string
	Population       int
	Elevation        int
	Dem              int
	Timezone         string
	ModificationDate string
	// This not included in the original data set
	Geohash             string
	AlternateNamesSlice []string
}

// MaxMind World City
type MaxMindCity struct {
	Country    string
	City       string
	AccentCity string
	Region     string
	Population int
	Latitude   float64
	Longitude  float64
	// This not included in the original data set
	Geohash string
}

var maxMindCityDedupeIdx map[string][]string

// Not currently used because world cities is a bigger set from Maxmind (and includes population)
// TODO: If world cities proves to use too much memory, then maybe fall back to this or provide an option in the NewGeobed() so the user can choose the trade off.
type MaxMindCityLite struct {
	LocId      int
	Country    string
	Region     string
	City       string
	PostalCode string
	Latitude   float64
	Longitude  float64
	MetroCode  string
	AreaCode   string
	// This not included in the original data set
	Geohash string
}

// Information about each country from Geonames including; ISO codes, FIPS, country capital, area (sq km), population, and more.
// Particularly useful for validating a location string contains a country name which can help the search process.
// Adding to this info, a slice of partial geohashes to help narrow down reverse geocoding lookups (maps to country buckets).
type CountryInfo struct {
	ISO                string
	ISO3               string
	ISONumeric         int
	Fips               string
	Country            string
	Capital            string
	Area               int
	Population         int
	Continent          string
	Tld                string
	CurrencyCode       string
	CurrencyName       string
	Phone              string
	PostalCodeFormat   string
	PostalCodeRegex    string
	Languages          string
	GeonameId          int
	Neighbours         string
	EquivalentFipsCode string
}

// Creates a new Geobed instance. You do not need more than one. You do not want more than one. There's a fair bit of data to load into memory.
func NewGeobed() GeoBed {
	g := GeoBed{}

	var err error
	g.c, err = loadGeobedCityData()
	g.co, err = loadGeobedCountryData()
	if err != nil || len(g.c) == 0 {
		g.downloadDataSets()
		g.loadDataSets()
		g.store()
	}

	return g
}

// Downloads the data sets if needed.
func (g *GeoBed) downloadDataSets() {
	os.Mkdir("./geobed-data", 0777)
	for _, f := range dataSetFiles {
		_, err := os.Stat(f["path"])
		if err != nil {
			if os.IsNotExist(err) {
				log.Println(f["path"] + " does not exist, downloading...")
				out, oErr := os.Create(f["path"])
				defer out.Close()
				if oErr == nil {
					r, rErr := http.Get(f["url"])
					defer r.Body.Close()
					if rErr == nil {
						_, nErr := io.Copy(out, r.Body)
						if nErr != nil {
							log.Println("Failed to copy data file, it will be tried again on next application start.")
							// remove file so another attempt can be made, should something fail
							err = os.Remove(f["path"])
						}
						r.Body.Close()
					}
					out.Close()
				} else {
					log.Println(oErr)
				}
			}
		}
	}
}

// Unzips the data sets and loads the data.
func (g *GeoBed) loadDataSets() {
	for _, f := range dataSetFiles {
		// This one is zipped
		if f["id"] == "geonamesCities1000" {
			rz, err := zip.OpenReader(f["path"])
			if err != nil {
				log.Fatal(err)
			}
			defer rz.Close()

			// Iterate through the files in the archive,
			// printing some of their contents.
			for _, uF := range rz.File {
				fi, err := uF.Open()

				if err != nil {
					log.Fatal(err)
				}
				defer fi.Close()

				//var allGeonamesCities []GeonamesCity

				// Geonames uses a tab delineated format and it's not even consistent. No CSV reader that I've found for Go can understand this.
				// I'm not expecting any reader to either because it's an invalid CSV to be frank. However, we can still split up each row by \t
				scanner := bufio.NewScanner(fi)
				scanner.Split(bufio.ScanLines)

				i := 1
				for scanner.Scan() {
					i++

					// So regexp, sadly, must be used (well, unless I wanted parse each string byte by byte, pushing each into a buffer to append to a slice until a tab is reached, etc.).
					// But I'd have to also then put in a condition if the next byte was a \t rune, then append an empty string, etc. This just, for now, seems nicer (easier).
					// This is only an import/update, so it shouldn't be an issue for performance. If it is, then I'll look into other solutions.
					fields := regexp.MustCompile("\t").Split(scanner.Text(), 19)

					// NOTE: Now using a combined GeobedCity struct since not all data sets have the same fields.
					// Plus, the entire point was to geocode forward and reverse. Bonus information like elevation and such is just superfluous.
					// Leaving it here because it may be configurable... If options are passed to NewGeobed() then maybe Geobed can simply be a Geonames search.
					// Don't even load in MaxMind data...And if that's the case, maybe that bonus information is desired.
					if len(fields) == 19 {
						//id, _ := strconv.Atoi(fields[0])
						lat, _ := strconv.ParseFloat(fields[4], 64)
						lng, _ := strconv.ParseFloat(fields[5], 64)
						pop, _ := strconv.Atoi(fields[14])
						//elv, _ := strconv.Atoi(fields[15])
						//dem, _ := strconv.Atoi(fields[16])

						gh := geohash.Encode(lat, lng)
						// This is produced with empty lat/lng values - don't store it.
						if gh == "7zzzzzzzzzzz" {
							gh = ""
						}

						// var gnc GeonamesCity
						// gnc.GeonameId = int(id)
						// gnc.Name = string(fields[1])
						// gnc.AsciiName = string(fields[2])
						// gnc.AlternateNames = string(fields[3])
						// gnc.AlternateNamesSlice = strings.Split(fields[3], ",")
						// gnc.Latitude = float64(lat)
						// gnc.Longitude = float64(lng)
						// gnc.FeatureClass = string(fields[6])
						// gnc.FeatureCode = string(fields[7])
						// gnc.CountryCode = string(fields[8])
						// gnc.CC2 = string(fields[9])
						// gnc.Admin1Code = string(fields[10])
						// gnc.Admin2Code = string(fields[11])
						// gnc.Admin3Code = string(fields[12])
						// gnc.Admin4Code = string(fields[13])
						// gnc.Population = int(pop)
						// gnc.Elevation = int(elv)
						// gnc.Dem = int(dem)
						// gnc.Timezone = string(fields[17])
						// gnc.ModificationDate = string(fields[18])
						// gnc.Geohash = gh
						// g.gc = append(g.gc, gnc)

						var c GeobedCity
						c.City = string(fields[1])
						c.CityAlt = string(fields[3])
						c.Country = string(fields[8])
						c.Region = string(fields[10])
						c.Latitude = lat
						c.Longitude = lng
						c.Population = int(pop)
						c.Geohash = gh
						g.c = append(g.c, c)
					}
				}
				// log.Println("Lines processed: ")
				// log.Println(i)
				// log.Println("Geoname cities added: ")
				// log.Println(len(g.gc))
				// log.Println("-----------------")
			}
		}

		// ...And this one is Gzipped
		if f["id"] == "maxmindWorldCities" {
			// It also has a lot of dupes
			maxMindCityDedupeIdx = make(map[string][]string)

			fi, err := os.Open(f["path"])
			if err != nil {
				log.Println(err)
			}
			defer fi.Close()

			fz, err := gzip.NewReader(fi)
			if err != nil {
				log.Println(err)
			}
			defer fz.Close()

			scanner := bufio.NewScanner(fz)
			scanner.Split(bufio.ScanLines)

			i := 1
			for scanner.Scan() {
				i++
				t := scanner.Text()

				// This may be the only one that would have actualled been a CSV, but reading line by line is ok.
				fields := strings.Split(t, ",")
				if len(fields) == 7 {
					var b bytes.Buffer
					b.WriteString(fields[0])
					b.WriteString(fields[1])
					b.WriteString(fields[4])

					idx := b.String()
					b.Reset()
					maxMindCityDedupeIdx[idx] = fields
				}
			}

			// Loop the map of fields after dupes have been removed (about 1/5th less... 2.6m vs 3.1m inreases lookup performance).
			for _, fields := range maxMindCityDedupeIdx {
				if fields[0] != "" && fields[0] != "0" {
					if fields[2] != "AccentCity" {
						pop, _ := strconv.Atoi(fields[4])
						lat, _ := strconv.ParseFloat(fields[5], 64)
						lng, _ := strconv.ParseFloat(fields[6], 64)

						gh := geohash.Encode(lat, lng)
						// This is produced with empty lat/lng values - don't store it.
						if gh == "7zzzzzzzzzzz" {
							gh = ""
						}

						// var mmc MaxMindCity
						// mmc.Country = toUpper(string(fields[0]))
						// mmc.City = string(fields[1])
						// mmc.AccentCity = string(fields[2])
						// mmc.Region = string(fields[3])
						// mmc.Population = int(pop)
						// mmc.Latitude = float64(lat)
						// mmc.Longitude = float64(lng)
						// mmc.Geohash = gh
						// g.mc = append(g.mc, mmc)

						var c GeobedCity
						c.City = string(fields[2])
						c.Country = toUpper(string(fields[0]))
						c.Region = string(fields[3])
						c.Latitude = lat
						c.Longitude = lng
						c.Population = int(pop)
						c.Geohash = gh
						g.c = append(g.c, c)
					}
				}
			}
			// Clear out the temrporary index
			maxMindCityDedupeIdx = make(map[string][]string)

			// log.Println("Lines process: ")
			// log.Println(i)
			// log.Println("MaxMind Cities added: ")
			// log.Println(len(g.mc))
			// log.Println("-----------------------")
		}

		// ...And this one is just plain text
		if f["id"] == "geonamesCountryInfo" {
			fi, err := os.Open(f["path"])

			if err != nil {
				log.Fatal(err)
			}
			defer fi.Close()

			scanner := bufio.NewScanner(fi)
			scanner.Split(bufio.ScanLines)

			i := 1
			for scanner.Scan() {
				t := scanner.Text()
				// There are a bunch of lines in this file that are comments, they start with #
				if string(t[0]) != "#" {
					i++
					fields := regexp.MustCompile("\t").Split(t, 19)

					if len(fields) == 19 {
						if fields[0] != "" && fields[0] != "0" {
							isoNumeric, _ := strconv.Atoi(fields[2])
							area, _ := strconv.Atoi(fields[6])
							pop, _ := strconv.Atoi(fields[7])
							gid, _ := strconv.Atoi(fields[16])

							var ci CountryInfo
							ci.ISO = string(fields[0])
							ci.ISO3 = string(fields[1])
							ci.ISONumeric = int(isoNumeric)
							ci.Fips = string(fields[3])
							ci.Country = string(fields[4])
							ci.Capital = string(fields[5])
							ci.Area = int(area)
							ci.Population = int(pop)
							ci.Continent = string(fields[8])
							ci.Tld = string(fields[9])
							ci.CurrencyCode = string(fields[10])
							ci.CurrencyName = string(fields[11])
							ci.Phone = string(fields[12])
							ci.PostalCodeFormat = string(fields[13])
							ci.PostalCodeRegex = string(fields[14])
							ci.Languages = string(fields[15])
							ci.GeonameId = int(gid)
							ci.Neighbours = string(fields[17])
							ci.EquivalentFipsCode = string(fields[18])

							g.co = append(g.co, ci)
						}
					}
				}
			}
		}

	}
}

// Forward geocode, location string to lat/lng (returns a struct though)
func (g *GeoBed) Geocode(n string) GeobedCity {
	var c GeobedCity
	n = strings.TrimSpace(n)
	if n == "" {
		return c
	}

	// Super naive way of finding the best match(es).
	// I tried belve (a search engine) and indexing. Not feasible, takes too long to index.
	// I tried splitting up cities into country buckets to reduce the time spent looping. Also not feasible because it's difficult to parse a location string and reliably get the elements.
	nSlice := strings.Split(n, " ")

	// Convert country to country code and add it to the search
	for _, co := range g.co {
		if strings.Contains(toLower(n), toLower(co.Country)) {
			nSlice = append(nSlice, co.ISO)
		}
	}

	var bestMatchingKeys = map[int]int{}
	var bestMatchingKey = 0
	for k, v := range g.c {
		// Exact match (with two or more fields) can return immediately. High enough confidence.
		if strings.EqualFold(n, v.City+", "+v.Region) || strings.EqualFold(n, v.City+" "+v.Region) {
			return v
		}

		// NOTE: HasPrefix() the other way around here because this is the unparsed query n string (which is longer than the city part of the GeobedCity record).
		// This looks for the passed in query to have the city first.
		if strings.HasPrefix(n, v.City) {
			if val, ok := bestMatchingKeys[k]; ok {
				bestMatchingKeys[k] = val + 4
			} else {
				bestMatchingKeys[k] = 4
			}
		}

		if v.CityAlt != "" {
			if strings.Contains(v.CityAlt, n) {
				if val, ok := bestMatchingKeys[k]; ok {
					bestMatchingKeys[k] = val + 1
				} else {
					bestMatchingKeys[k] = 1
				}
			}
		}

		// Exact match (city only) -- does this even make sense? it's a pretty big guess
		// It catches things like "New York" ... Because otherwise "New" is found (which is apparently a city) and scores higher...
		// Though every check adds a little bit of time to the process.
		if strings.EqualFold(n, v.City) {
			if val, ok := bestMatchingKeys[k]; ok {
				bestMatchingKeys[k] = val + 6
			} else {
				bestMatchingKeys[k] = 6
			}
		}

		for _, ns := range nSlice {
			ns = strings.TrimSuffix(ns, ",")

			// City (here we are looking to see if a piece in the query n string is a prefix to the city part of the GeobedCity record)
			if strings.HasPrefix(v.City, ns) {
				if val, ok := bestMatchingKeys[k]; ok {
					bestMatchingKeys[k] = val + 1
				} else {
					bestMatchingKeys[k] = 1
				}
			}

			if strings.Contains(v.CityAlt, ns) {
				if val, ok := bestMatchingKeys[k]; ok {
					bestMatchingKeys[k] = val + 1
				} else {
					bestMatchingKeys[k] = 1
				}
			}

			// City (worth 2 points if exact match - note: this doesn't work so well for cities with multiple words)
			if strings.EqualFold(v.City, ns) {
				if val, ok := bestMatchingKeys[k]; ok {
					bestMatchingKeys[k] = val + 2
				} else {
					bestMatchingKeys[k] = 2
				}
			}

			// Region (state/province)
			if strings.EqualFold(v.Region, ns) {
				if val, ok := bestMatchingKeys[k]; ok {
					bestMatchingKeys[k] = val + 3
				} else {
					bestMatchingKeys[k] = 3
				}
			}
			// Country (worth 2 points if exact match)
			if strings.EqualFold(v.Country, ns) {
				if val, ok := bestMatchingKeys[k]; ok {
					bestMatchingKeys[k] = val + 2
				} else {
					bestMatchingKeys[k] = 2
				}
			}
		}
	}

	m := 0
	for k, v := range bestMatchingKeys {
		if v > m {
			m = v
			bestMatchingKey = k
		}
		// If there is a tie breaker, use the city with the higher population (if known) because it's more likely to be what is meant.
		// For example, when people say "New York" they typically mean New York, NY...Though there are many New Yorks.
		if v == m {
			if g.c[k].Population > g.c[bestMatchingKey].Population {
				bestMatchingKey = k
			}
		}
	}

	// log.Println("Possible results:")
	// log.Println(len(bestMatchingKeys))
	// log.Println("Best match:")
	// log.Println(g.c[bestMatchingKey])
	// log.Println("Scored:")
	// log.Println(m)

	c = g.c[bestMatchingKey]
	return c
}

// Reverse geocode
func (g *GeoBed) ReverseGeocode(lat float64, lng float64) GeobedCity {
	c := GeobedCity{}

	gh := geohash.Encode(lat, lng)
	// This is produced with empty lat/lng values - don't look for anything.
	if gh == "7zzzzzzzzzzz" {
		return c
	}

	// Note: All geohashes are going to be 12 characters long. Even if the precision on the lat/lng isn't great. The geohash package will center things.
	// Obviously lat/lng like 37, -122 is a guess. That's no where near the resolution of a city. Though we're going to allow guesses.
	mostMatched := 0
	matched := 0
	for k, v := range g.c {
		// check first two characters to reduce the number of loops
		if v.Geohash[0] == gh[0] && v.Geohash[1] == gh[1] {
			matched = 2
			for i := 2; i <= len(gh); i++ {
				//log.Println(gh[0:i])
				if v.Geohash[0:i] == gh[0:i] {
					matched++
				}
			}
			// tie breakers go to city with larger population (NOTE: There's still a chance that the next pass will uncover a better match)
			if matched == mostMatched && g.c[k].Population > c.Population {
				c = g.c[k]
				// log.Println("MATCHES")
				// log.Println(matched)
				// log.Println("CITY")
				// log.Println(c.City)
				// log.Println("POPULATION")
				// log.Println(c.Population)
			}
			if matched > mostMatched {
				c = g.c[k]
				mostMatched = matched
			}
		}
	}

	return c
}

// A slightly faster lowercase function.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range b {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// A slightly faster uppercase function.
func toUpper(s string) string {
	b := make([]byte, len(s))
	for i := range b {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// Dumps the Geobed data to disk. This speeds up startup time on subsequent runs (or if calling NewGeobed() multiple times which should be avoided if possible).
// TODO: Refactor
func (g GeoBed) store() error {
	b := new(bytes.Buffer)

	// Store the city info
	enc := gob.NewEncoder(b)
	err := enc.Encode(g.c)
	if err != nil {
		b.Reset()
		return err
	}

	fh, eopen := os.OpenFile("./geobed-data/g.c.dmp", os.O_CREATE|os.O_WRONLY, 0666)
	defer fh.Close()
	if eopen != nil {
		b.Reset()
		return eopen
	}
	n, e := fh.Write(b.Bytes())
	if e != nil {
		b.Reset()
		return e
	}
	log.Printf("%d bytes successfully written to file\n", n)

	// Store the country info as well (this is all now repetition - refactor)
	b.Reset()
	enc = gob.NewEncoder(b)
	err = enc.Encode(g.co)
	if err != nil {
		b.Reset()
		return err
	}

	fh, eopen = os.OpenFile("./geobed-data/g.co.dmp", os.O_CREATE|os.O_WRONLY, 0666)
	defer fh.Close()
	if eopen != nil {
		b.Reset()
		return eopen
	}
	n, e = fh.Write(b.Bytes())
	if e != nil {
		b.Reset()
		return e
	}
	log.Printf("%d bytes successfully written to file\n", n)

	b.Reset()
	return nil
}

// Loads a GeobedCity dump, which saves a bit of time.
func loadGeobedCityData() ([]GeobedCity, error) {
	fh, err := os.Open("./geobed-data/g.c.dmp")
	if err != nil {
		return nil, err
	}
	gc := []GeobedCity{}
	dec := gob.NewDecoder(fh)
	err = dec.Decode(&gc)
	if err != nil {
		return nil, err
	}
	return gc, nil
}

func loadGeobedCountryData() ([]CountryInfo, error) {
	fh, err := os.Open("./geobed-data/g.co.dmp")
	if err != nil {
		return nil, err
	}
	co := []CountryInfo{}
	dec := gob.NewDecoder(fh)
	err = dec.Decode(&co)
	if err != nil {
		return nil, err
	}
	return co, nil
}

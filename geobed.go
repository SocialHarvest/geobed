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
	"sort"
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
	c  Cities
	co []CountryInfo
}

type Cities []GeobedCity

func (c Cities) Len() int {
	return len(c)
}
func (c Cities) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}
func (c Cities) Less(i, j int) bool {
	return c[i].CityLower < c[j].CityLower
}

// A combined city struct (the various data sets have different fields, this combines what's available and keeps things smaller).
type GeobedCity struct {
	City string
	// This lowercase version is specifically to make sorting and searching faster
	CityLower  string
	CityAlt    string
	Country    string
	Region     string
	Latitude   float64
	Longitude  float64
	Population int32
	Geohash    string
}

var maxMindCityDedupeIdx map[string][]string

// Holds information about the index ranges for city names (1st and 2nd characters) to help narrow down sets of the GeobedCity slice to scan when looking for a match.
var cityNameIdx map[string]int

// Information about each country from Geonames including; ISO codes, FIPS, country capital, area (sq km), population, and more.
// Particularly useful for validating a location string contains a country name which can help the search process.
// Adding to this info, a slice of partial geohashes to help narrow down reverse geocoding lookups (maps to country buckets).
type CountryInfo struct {
	Country            string
	Capital            string
	Area               int32
	Population         int32
	GeonameId          int32
	ISONumeric         int16
	ISO                string
	ISO3               string
	Fips               string
	Continent          string
	Tld                string
	CurrencyCode       string
	CurrencyName       string
	Phone              string
	PostalCodeFormat   string
	PostalCodeRegex    string
	Languages          string
	Neighbours         string
	EquivalentFipsCode string
}

// Creates a new Geobed instance. You do not need more than one. You do not want more than one. There's a fair bit of data to load into memory.
func NewGeobed() GeoBed {
	g := GeoBed{}

	var err error
	g.c, err = loadGeobedCityData()
	g.co, err = loadGeobedCountryData()
	err = loadGeobedCityNameIdx()
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

			for _, uF := range rz.File {
				fi, err := uF.Open()

				if err != nil {
					log.Fatal(err)
				}
				defer fi.Close()

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

						var c GeobedCity
						c.City = strings.Trim(string(fields[1]), " ")
						c.CityLower = toLower(c.City)
						c.CityAlt = string(fields[3])
						c.Country = string(fields[8])
						c.Region = string(fields[10])
						c.Latitude = lat
						c.Longitude = lng
						c.Population = int32(pop)
						c.Geohash = gh

						// Don't include entries without a city name. If we want to geocode the centers of countries and states, then we can do that faster through other means.
						if len(c.City) > 0 {
							g.c = append(g.c, c)
						}
					}
				}
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
						// MaxMind's data set is a bit dirty. I've seen city names surrounded by parenthesis in a few places.
						cn := strings.Trim(string(fields[2]), " ")
						cn = strings.Trim(cn, "( )")

						// Don't take any city names with erroneous punctuation either.
						if strings.Contains(cn, "!") || strings.Contains(cn, "@") {
							continue
						}

						gh := geohash.Encode(lat, lng)
						// This is produced with empty lat/lng values - don't store it.
						if gh == "7zzzzzzzzzzz" {
							gh = ""
						}

						var c GeobedCity
						c.City = cn
						c.CityLower = toLower(c.City)
						c.Country = toUpper(string(fields[0]))
						c.Region = string(fields[3])
						c.Latitude = lat
						c.Longitude = lng
						c.Population = int32(pop)
						c.Geohash = gh

						// Don't include entries without a city name. If we want to geocode the centers of countries and states, then we can do that faster through other means.
						if len(c.City) > 0 {
							g.c = append(g.c, c)
						}
					}
				}
			}
			// Clear out the temrporary index (set to nil, it does get re-created) so that Go can garbage collect it at some point whenever it feels the need.
			maxMindCityDedupeIdx = nil
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
							ci.ISONumeric = int16(isoNumeric)
							ci.Fips = string(fields[3])
							ci.Country = string(fields[4])
							ci.Capital = string(fields[5])
							ci.Area = int32(area)
							ci.Population = int32(pop)
							ci.Continent = string(fields[8])
							ci.Tld = string(fields[9])
							ci.CurrencyCode = string(fields[10])
							ci.CurrencyName = string(fields[11])
							ci.Phone = string(fields[12])
							ci.PostalCodeFormat = string(fields[13])
							ci.PostalCodeRegex = string(fields[14])
							ci.Languages = string(fields[15])
							ci.GeonameId = int32(gid)
							ci.Neighbours = string(fields[17])
							ci.EquivalentFipsCode = string(fields[18])

							g.co = append(g.co, ci)
						}
					}
				}
			}
		}
	}

	// Sort []GeobedCity by city names to help with binary search (the City field is the most searched upon field and the matching names can be easily filtered down from there).
	sort.Sort(g.c)

	// Index the locations of city names in the g.c []GeoCity slice. This way when searching the range can be limited so it will be faster.
	cityNameIdx = make(map[string]int)
	for k, v := range g.c {
		// Get the index key for the first character of the city name.
		ik := string(v.CityLower[0])
		if val, ok := cityNameIdx[ik]; ok {
			// If this key number is greater than what was previously recorded, then set it as the new indexed key.
			if val < k {
				cityNameIdx[ik] = k
			}
		} else {
			// If the index key has not yet been set for this value, then set it.
			cityNameIdx[ik] = k
		}

		// Get the index key for the first two characters of the city name.
		if len(v.CityLower) >= 2 {
			ik2 := v.CityLower[0:2]
			if val, ok := cityNameIdx[ik2]; ok {
				// If this key number is greater than what was previously recorded, then set it as the new indexed key.
				if val < k {
					cityNameIdx[ik2] = k
				}
			} else {
				// If the index key has not yet been set for this value, then set it.
				cityNameIdx[ik2] = k
			}
		}
	}
}

// Forward geocode, location string to lat/lng (returns a struct though)
func (g *GeoBed) Geocode(n string) GeobedCity {
	var c GeobedCity
	var re = regexp.MustCompile("")
	n = strings.TrimSpace(n)
	if n == "" {
		return c
	}

	// Extract all potential abbreviations.
	re = regexp.MustCompile(`[\S]{2,3}`)
	abbrevSlice := re.FindStringSubmatch(n)

	// Convert country to country code and pull it out. We'll use it as a secondary form of validation. Remove the code from the original query.
	nCo := ""
	for _, co := range g.co {
		re = regexp.MustCompile("(?i)^" + co.Country + ",?\\s|\\s" + co.Country + ",?\\s" + co.Country + "\\s$")
		if re.MatchString(n) {
			nCo = co.ISO
			// And remove it so we have a cleaner query string for a city.
			n = re.ReplaceAllString(n, "")
		}
	}

	// Find US State codes and pull them out as well (do not convert state names, they can also easily be city names).
	nSt := ""
	for sc, _ := range UsSateCodes {
		re = regexp.MustCompile("(?i)^" + sc + ",?\\s|\\s" + sc + ",?\\s|\\s" + sc + "$")
		if re.MatchString(n) {
			nSt = sc
			// And remove it too.
			n = re.ReplaceAllString(n, "")
		}
	}
	// Trim spaces and commas off the modified string.
	n = strings.Trim(n, " ,")

	// Now extract words (potential city names) into a slice. With this, the index will be referenced to pinpoint sections of the g.c []GeobedCity slice to scan.
	// This results in a much faster lookup. This is over a simple binary search with strings.Search() etc. because the city name may not be the first word.
	// This should not contain any known country code or US state codes.
	nSlice := strings.Split(n, " ")

	// Figure out the keys we should range over. This reduces the size of the slice to ultimately range over and increases lookup time.
	// A binary search could not really be used because data is dirty. Was that query just the city name? Great! We could use a binary search...
	// But what if the query contained more? A state? Country? Maybe an alternate name for a city... It's just not going to work.
	// However, we can still use other methods to narrow down the lookup and that's what cityNameIdx is all about.
	type r struct {
		f int
		t int
	}
	ranges := []r{}
	for _, ns := range nSlice {
		ns = strings.TrimSuffix(ns, ",")

		if len(ns) > 0 {
			// Get the first character in the string, this tells us where to stop.
			fc := toLower(string(ns[0]))
			// Get the previous index key (by getting the previous character in the alphabet) to figure out where to start.
			pik := string(prev(rune(fc[0])))

			// To/from key
			fk := 0
			tk := 0
			if val, ok := cityNameIdx[pik]; ok {
				fk = val
			}
			if val, ok := cityNameIdx[fc]; ok {
				tk = val
			}
			// Don't let the to key be out of range.
			if tk == 0 {
				tk = (len(g.c) - 1)
			}
			ranges = append(ranges, r{fk, tk})
		}
	}

	var bestMatchingKeys = map[int]int{}
	var bestMatchingKey = 0
	for _, rng := range ranges {
		// When adjusting the range, the keys become out of sync. Start from rng.f
		currentKey := rng.f

		for _, v := range g.c[rng.f:rng.t] {
			currentKey++

			// Mainly useful for strings like: "Austin, TX" or "Austin TX" (locations with US state codes). Smile if your location string is this simple.
			if nSt != "" {
				if strings.EqualFold(n, v.City) && strings.EqualFold(nSt, v.Region) {
					return v
				}
			}

			// Special case. Airport codes and other short 3 letter abbreviations, ie. NYC and SFO
			// Country codes could present problems here. It seems to work for NYC, but not SFO (which there are multiple SFOs actually).
			// Leaving it for now, but airport codes are tricky (though they are popular on Twitter). These must be exact (case sensitive) matches.
			// if len(n) == 3 {
			// 	alts := strings.Split(v.CityAlt, ",")
			// 	for _, altV := range alts {
			// 		if altV != "" {
			// 			if altV == n {
			// 				if val, ok := bestMatchingKeys[currentKey]; ok {
			// 					bestMatchingKeys[currentKey] = val + 4
			// 				} else {
			// 					bestMatchingKeys[currentKey] = 4
			// 				}
			// 			}
			// 		}
			// 	}
			// }

			// Abbreviations for state/country
			// Region (state/province)
			for _, av := range abbrevSlice {
				lowerAv := toLower(av)
				if len(av) == 2 && strings.EqualFold(v.Region, lowerAv) {
					if val, ok := bestMatchingKeys[currentKey]; ok {
						bestMatchingKeys[currentKey] = val + 5
					} else {
						bestMatchingKeys[currentKey] = 5
					}
				}

				// Country (worth 2 points if exact match)
				if len(av) == 2 && strings.EqualFold(v.Country, lowerAv) {
					if val, ok := bestMatchingKeys[currentKey]; ok {
						bestMatchingKeys[currentKey] = val + 3
					} else {
						bestMatchingKeys[currentKey] = 3
					}
				}
			}

			// A discovered country name converted into a country code
			if nCo != "" {
				if nCo == v.Country {
					if val, ok := bestMatchingKeys[currentKey]; ok {
						bestMatchingKeys[currentKey] = val + 4
					} else {
						bestMatchingKeys[currentKey] = 4
					}
				}
			}

			// A discovered state name converted into a region code
			if nSt != "" {
				if nSt == v.Region {
					if val, ok := bestMatchingKeys[currentKey]; ok {
						bestMatchingKeys[currentKey] = val + 4
					} else {
						bestMatchingKeys[currentKey] = 4
					}
				}
			}

			// If any alternate names can be discovered, take them into consideration.
			if v.CityAlt != "" {
				alts := strings.Fields(v.CityAlt)
				for _, altV := range alts {
					if strings.EqualFold(altV, n) {
						if val, ok := bestMatchingKeys[currentKey]; ok {
							bestMatchingKeys[currentKey] = val + 3
						} else {
							bestMatchingKeys[currentKey] = 3
						}
					}
					// Exact, a case-sensitive match means a lot.
					if altV == n {
						if val, ok := bestMatchingKeys[currentKey]; ok {
							bestMatchingKeys[currentKey] = val + 5
						} else {
							bestMatchingKeys[currentKey] = 5
						}
					}
				}
			}

			// Exact city name matches mean a lot.
			if strings.EqualFold(n, v.City) {
				if val, ok := bestMatchingKeys[currentKey]; ok {
					bestMatchingKeys[currentKey] = val + 7
				} else {
					bestMatchingKeys[currentKey] = 7
				}
			}

			for _, ns := range nSlice {
				ns = strings.TrimSuffix(ns, ",")

				// City (worth 2 points if contians part of string)
				if strings.Contains(toLower(v.City), toLower(ns)) {
					if val, ok := bestMatchingKeys[currentKey]; ok {
						bestMatchingKeys[currentKey] = val + 2
					} else {
						bestMatchingKeys[currentKey] = 2
					}
				}

				// If there's an exat match, maybe there was noise in the string so it could be the full city name, but unlikely. For example, "New" or "Los" is in many city names.
				// Still, give it a point because it could be the bulkier part of a city name (or the city name could be one word). This has helped in some cases.
				if strings.EqualFold(v.City, ns) {
					if val, ok := bestMatchingKeys[currentKey]; ok {
						bestMatchingKeys[currentKey] = val + 1
					} else {
						bestMatchingKeys[currentKey] = 1
					}
				}

			}
		}
	}

	// If no country was found, look at population as a factor. Is it obvious?
	if nCo == "" {
		hp := int32(0)
		hpk := 0
		for k, v := range bestMatchingKeys {
			// Add bonus point for having a population 1,000+
			if g.c[k].Population >= 1000 {
				bestMatchingKeys[k] = v + 1
			}
			// Now just add a bonus for having the highest population and points
			if g.c[k].Population > hp {
				hpk = k
				hp = g.c[k].Population
			}
		}
		// Add a point for having the highest population (if any of the results had population data available).
		if g.c[hpk].Population > 0 {
			bestMatchingKeys[hpk] = bestMatchingKeys[hpk] + 1
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
	// for _, kv := range bestMatchingKeys {
	// 	log.Println(g.c[kv])
	// }
	// log.Println("Best match:")
	// log.Println(g.c[bestMatchingKey])
	// log.Println("Scored:")
	// log.Println(m)

	c = g.c[bestMatchingKey]
	return c
}

func prev(r rune) rune {
	return r - 1
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
	//enc = gob.NewEncoder(b)
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

	// Store the index info (again there's some repetition here)
	b.Reset()
	//enc = gob.NewEncoder(b)
	err = enc.Encode(cityNameIdx)
	if err != nil {
		b.Reset()
		return err
	}

	fh, eopen = os.OpenFile("./geobed-data/cityNameIdx.dmp", os.O_CREATE|os.O_WRONLY, 0666)
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

func loadGeobedCityNameIdx() error {
	fh, err := os.Open("./geobed-data/cityNameIdx.dmp")
	if err != nil {
		return err
	}
	dec := gob.NewDecoder(fh)
	cityNameIdx = make(map[string]int)
	err = dec.Decode(&cityNameIdx)
	if err != nil {
		return err
	}
	return nil
}

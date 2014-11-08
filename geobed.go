package main

import (
	"archive/zip"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	//"encoding/csv"
	"bufio"
	//csv "github.com/tmaiaroto/gocsv"
	geohash "github.com/TomiHiltunen/geohash-golang"
	fuzzy "github.com/sajari/fuzzy"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// There are over 2.4 million cities in the world. The Geonames data set only contains 143,270 and the MaxMind set contains 567,382. Obviously there's a lot of overlap.
// It may not be possible to have information for all cities, but many of the cities are also fairly remote and likely don't have internet access anyway.
// The Geonames data is preferred because it contains additional information such as elevation, population, and more. Population is good particuarly nice because a sense for
// the city size can be understood by applications. So showing all major cities is pretty easy. Though the primary goal of this package is to geocode, the additional information
// is bonus. So after checking the Geonames set, the geocoding functions will then look at MaxMind's.
// Maybe in the future this package will even use the Geonames premium data and have functions to look up nearest airports, etc.
// I would simply use just Geonames data, but there's so many more cities in the MaxMind set despite the lack of additional details.
//
// http://download.geonames.org/export/dump/cities1000.zip
// http://geolite.maxmind.com/download/geoip/database/GeoLiteCity_CSV/GeoLiteCity-latest.zip

// A list of data sources. I'm not sure why I bothered with the "format" key, it's not like I found any two sources that followed the same format...But it does let me know how to parse them.
var dataSetFiles = []map[string]string{
	{"url": "http://download.geonames.org/export/dump/cities1000.zip", "path": "./geobed-data/cities1000.zip", "downloadType": "zip", "format": "geonames"},
	{"url": "http://download.geonames.org/export/dump/countryInfo.txt", "path": "./geobed-data/countryInfo.txt", "downloadType": "txt", "format": "geonames"},
	//{"url": "http://geolite.maxmind.com/download/geoip/database/GeoLiteCity_CSV/GeoLiteCity-latest.zip", "path": "./geobed-data/GeoLiteCity-latest.zip", "downloadType": "zip", "format": "csv"},
	// http://download.maxmind.com/download/worldcities/worldcitiespop.txt.gz // this would then have the population details and lat/lng ... but it's over 100MB unzipped and contains dupes.
	// dupes are easily mitigated when we load the data in, but what does it do for memory usage now? thats a big file...
	// it does not have zip codes, but it does contain more cities than geonames or the geolitecity database.
	// https://www.maxmind.com/en/worldcities
}

// A handy map of US state codes to full names
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

type GeoBed struct {
	db  *sqlx.DB
	gc  []GeonamesCity
	mc  []MaxMindCity
	mcl []MaxMindCityLite
	co  []CountryInfo
}

// Geonames City (with population of 1,000 or greater)
type GeonamesCity struct {
	GeonameId int64  `csv: "geonameid", db: "geonameid"`
	Name      string `csv: "name", db: "name"`
	AsciiName string `csv: "asciiname", db: "asciiname"`
	// This might make sense as a slice, but then what? JOINs in the database? Multiple records? No, this is still easy to search for.
	AlternateNames      string
	AlternateNamesSlice []string
	Latitude            float64
	Longitude           float64
	FeatureClass        string
	FeatureCode         string
	CountryCode         string
	CC2                 string
	Admin1Code          string
	Admin2Code          string
	Admin3Code          string
	Admin4Code          string
	Population          int64
	Elevation           int64
	Dem                 int64
	Timezone            string
	ModificationDate    string
	Geohash             string
}

// MaxMind World City
type MaxMindCity struct {
	Country    string
	City       string
	AccentCity string
	Region     string
	Population int64
	Latitude   float64
	Longitude  float64
}

// Not currently used because world cities is a bigger set from Maxmind (and includes population)
type MaxMindCityLite struct {
	LocId      int64
	Country    string
	Region     string
	City       string
	PostalCode string
	Latitude   float64
	Longitude  float64
	MetroCode  string
	AreaCode   string
}

// Information about each country from Geonames including; ISO codes, FIPS, country capital, area (sq km), population, and more.
// Particularly useful for validating a location string contains a country name which can help the search process.
type CountryInfo struct {
	ISO                string
	ISO3               string
	ISONumeric         string
	Fips               string
	CountryCapital     string
	Area               int64
	Population         int64
	Continent          string
	Tld                string
	CurrencyCode       string
	CurrencyName       string
	Phone              string
	PostalCodeFormat   string
	PostalCodeRegex    string
	Languages          string
	GeonameId          int64
	Neighbours         string
	EquivalentFipsCode string
}

func NewGeobed() GeoBed {
	g := GeoBed{}
	var err error

	// @see https://www.sqlite.org/uri.html
	// an in memory database would be nice, but the amount of data is a bit large...maybe add an option to use it instead.
	g.db, err = sqlx.Open("sqlite3", "file:geobed.db?cache=shared&mode=rwc")

	// Might actually be able to keep this all in memory... just read from the zip file, make the struct and loop it. Use string searches.
	schema := `CREATE TABLE IF NOT EXISTS cities (
    geonameid integer,
    name text NULL,
    asciiname text NULL,
	alternatenames text NULL,
	latitude real,
	longitude real,
	featureclass text NULL,
	featurecode text NULL,
	countrycode text NULL,
	cc2 text NULL,
	admin1code text NULL,
	admin2code text NULL,
	admin3code text NULL,
	admin4code text NULL,
	population integer,
	elevation integer,
	dem integer,
	timezone text NULL,
	modificationdate text NULL,
	geohash text NULL
    );`

	_, err = g.db.Exec(schema)
	if err != nil {
		log.Println(err)
	}

	return g
}

// Gets data from free data sets and inserts into the embedded database, used to initially get the data as well as retrieve updates in a redundant way
func populateDb() {

}

// Downloads the data sets
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

// Unzips the data sets
func (g *GeoBed) unzipDataSets() {
	for _, f := range dataSetFiles {
		if f["downloadType"] == "zip" {
			r, err := zip.OpenReader(f["path"])
			if err != nil {
				log.Fatal(err)
			}
			defer r.Close()

			// Iterate through the files in the archive,
			// printing some of their contents.
			for _, uF := range r.File {
				rc, err := uF.Open()

				if err != nil {
					log.Fatal(err)
				}
				defer rc.Close()

				if f["format"] == "geonames" {
					//var allGeonamesCities []GeonamesCity

					// Geonames uses a tab delineated format and it's not even consistent. No CSV reader that I've found for Go can understand this.
					// I'm not expecting any reader to either because it's an invalid CSV to be frank. However, we can still split up each row by \t
					scanner := bufio.NewScanner(rc)
					scanner.Split(bufio.ScanLines)

					i := 1
					for scanner.Scan() {
						i++
						// Won't work because it doesn't return empty strings.
						// fn := func(c rune) bool {
						// 	return c == '\t'
						// }
						// line := strings.FieldsFunc(scanner.Text(), fn)

						// So regexp, sadly, must be used (well, unless I wanted parse each string byte by byte, pushing each into a buffer to append to a slice until a tab is reached, etc.).
						// But I'd have to also then put in a condition if the next byte was a \t rune, then append an empty string, etc. This just, for now, seems nicer (easier).
						// This is only an import/update, so it shouldn't be an issue for performance. If it is, then I'll look into other solutions.
						line := regexp.MustCompile("\t").Split(scanner.Text(), 19)

						if len(line) == 19 {
							id, _ := strconv.Atoi(line[0])
							lat, _ := strconv.ParseFloat(line[4], 64)
							lng, _ := strconv.ParseFloat(line[5], 64)
							pop, _ := strconv.Atoi(line[14])
							elv, _ := strconv.Atoi(line[15])
							dem, _ := strconv.Atoi(line[16])

							gh := geohash.Encode(lat, lng)
							// This is produced with empty lat/lng values - don't store it.
							if gh == "7zzzzzzzzzzz" {
								gh = ""
							}

							var gnc GeonamesCity
							gnc.GeonameId = int64(id)
							gnc.Name = string(line[1])
							gnc.AsciiName = string(line[2])
							gnc.AlternateNames = string(line[3])
							gnc.AlternateNamesSlice = strings.Split(line[3], ",")
							gnc.Latitude = float64(lat)
							gnc.Longitude = float64(lng)
							gnc.FeatureClass = string(line[6])
							gnc.FeatureCode = string(line[7])
							gnc.CountryCode = string(line[8])
							gnc.CC2 = string(line[9])
							gnc.Admin1Code = string(line[10])
							gnc.Admin2Code = string(line[11])
							gnc.Admin3Code = string(line[12])
							gnc.Admin4Code = string(line[13])
							gnc.Population = int64(pop)
							gnc.Elevation = int64(elv)
							gnc.Dem = int64(dem)
							gnc.Timezone = string(line[17])
							gnc.ModificationDate = string(line[18])
							gnc.Geohash = gh

							//allGeonamesCities = append(allGeonamesCities, gnc)
							g.gc = append(g.gc, gnc)
						}
					}
					log.Println("Lines processed: ")
					log.Println(i)
					log.Println("-----------------")

					//log.Println(len(allGeonamesCities))
					log.Println(len(g.gc))
				}

				if f["format"] == "csv" {

					// br := bufio.NewReader(rc)
					// csvReader := csv.NewReader(br)
					// csvReader.Config.FieldDelim = '\t'
					// //csvReader.Config.TrimSpaces = true
					// all_recs, err := csvReader.ReadAll()
					// log.Println(err)
					// log.Println(len(all_recs))

					//all_recs, err := csv.ReadAll(rc) // gives a 2d slice of the file rc, each slice is a line split into numField sections

					// if err != nil {
					// 	rc.Close()
					// 	log.Println("error reading")
					// 	log.Println(err.Error())
					// }
					// locations := []GeonamesCities{}
					// //g := make(Graph)
					// //actors := make(map[string]bool) // create a map of actors, used later on to compute centralities
					// i := 1
					// for _, line := range all_recs {
					// 	if len(line) == 0 {
					// 		//log.Println(line)
					// 		//log.Println("empty")
					// 	}
					// 	if len(line) >= 18 {
					// 		//make_link(g, line[0], line[1]+" "+line[2])
					// 		//actors[line[0]] = true
					// 		//log.Println(line)
					// 		id, _ := strconv.Atoi(line[0])
					// 		//latString := line[4][0 : len(line[4])-2]
					// 		//lngString := line[5][0 : len(line[5])-2]
					// 		//log.Println(line[4])
					// 		//log.Println(line[5])

					// 		//lat, _ := strconv.ParseFloat(latString, 64)
					// 		//lng, _ := strconv.ParseFloat(lngString, 64)
					// 		population := 0
					// 		if line[14] != "" {
					// 			population, _ = strconv.Atoi(line[14])
					// 		}
					// 		elevation, _ := strconv.Atoi(line[15])
					// 		dem, _ := strconv.Atoi(line[16])

					// 		location := GeonamesCities{
					// 			GeonameId: int64(id),
					// 			// Name:           line[1],
					// 			// AsciiName:      line[2],
					// 			// AlternateNames: line[3],
					// 			// //Latitude:       lat,
					// 			// //Longitude:      lng,
					// 			// FeatureClass: line[6],
					// 			// FeatureCode:  line[7],
					// 			// CountryCode:  line[8],
					// 			// CC2:          line[9],
					// 			// Admin1Code:   line[10],
					// 			// Admin2Code:   line[11],
					// 			// Admin3Code:   line[12],
					// 			// Admin4Code:   line[13],
					// 			Population: int64(population),
					// 			Elevation:  int64(elevation),
					// 			Dem:        int64(dem),
					// 			// Timezone:     line[17],
					// 			// //ModificationDate: line[18],
					// 		}
					// 		//log.Println(line)
					// 		//log.Println(location)
					// 		locations = append(locations, location)
					// 	}
					// 	log.Println(len(locations))
					// 	log.Println(i)
					// 	i++
					// }
				}

				///
				// log.Printf("Contents of %s:\n", uF.Name)
				// rc, err := uF.Open()
				// if err != nil {
				// 	log.Fatal(err)
				// }
				// _, err = io.Copy(os.Stdout, rc, 68)
				// if err != nil {
				// 	log.Fatal(err)
				// }
				// rc.Close()
				///
			}
		}
	}
}

// Forward geocode, location string to lat/lng (returns a struct though)
func (g *GeoBed) Geocode(n string) GeonamesCity {
	var fd GeonamesCity
	r, err := regexp.Compile(`(?i)^` + n)

	// The passed location string may have with it a state or country, try to break it apart in some basic ways to figure out if so
	//nSlice := n.Split(n, ",")
	// for _, s := range nSlice {

	// }

	bestI := 0
	bestScore := 1000

	// TODO: Take country, state into account...

	for i, c := range g.gc {
		// Exact match (the best)
		if n == c.Name || n == c.AsciiName {
			//return g.gc[i]
		}
		// Exact match on AlternateNames
		for _, an := range c.AlternateNamesSlice {
			if n == an {
				return g.gc[i]
			}
		}

		// If the regex was valid, use it to search
		if err == nil {
			if r.MatchString(c.AlternateNames) == true {
				//return g.gc[i]
			}
		}

		// Best fuzzy match

		d := fuzzy.Levenshtein(&n, &c.AlternateNames)
		//log.Println(d)
		if d < bestScore {
			bestScore = d
			bestI = i
			log.Println("best score now:")
			log.Println(c.Name)
		}

	}
	return g.gc[bestI]
	return fd
}

func main() {
	//Debug - do not compile with this
	runtime.SetBlockProfileRate(1)
	// Start a profile server so information can be viewed using a web browser
	// go func() {
	// 	log.Println(http.ListenAndServe("localhost:6060", nil))
	// }()

	gb := NewGeobed()
	//log.Println(gb)
	gb.downloadDataSets()
	gb.unzipDataSets()

	found := gb.Geocode("Fura")
	log.Println(found)

	http.ListenAndServe("localhost:6060", nil)
}

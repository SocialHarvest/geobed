Geobed
============

[![Build Status](https://drone.io/github.com/SocialHarvest/geobed/status.png)](https://drone.io/github.com/SocialHarvest/geobed/latest) [![Coverage Status](https://coveralls.io/repos/SocialHarvest/geobed/badge.png)](https://coveralls.io/r/SocialHarvest/geobed)

This Golang package contains an embedded geocoder. There are no major external dependendies other than some downloaded data files. Once downloaded, those data files 
are stored in memory. So after the initial load there truly are no outside dependencies. It geocodes and reverse geocodes to a city level detail. It approximates and takes 
educated guesses when not enough detail is provided. See test cases for more examples.

## Why?

To keep it short and simple, the reason this package was built was because geocoding services are really expensive. If city level detail is enough and you don't need street addresses, 
then this should be completely fine. It's also nice that there are no HTTP requests being made to do this (after initial load - and the data files can be copied to other places).

Performance is pretty good, but that is one of the goals. Overtime it should improve, but for now it geocodes a string to lat/lng in about 0.0125 - 0.0135 seconds (on a Macbook Pro).

## Usage

You should re-use the ```GeoBed``` struct as it contains a LOT of data (2.7+ million items). On this struct are the functions to geocode and reverse geocode. Be aware that
this also means your machine will need a good bit of RAM since this is all data held in memory (which is also what makes it fast too).

```
g := NewGeobed()
c := g.Geocode("london")
```

In the above case, ```c``` should end up being:

```
{London london City of London,Gorad Londan,ILondon,LON,Lakana,Landen,Ljondan,Llundain,Londain,Londan,Londar,Londe,Londen,Londinium,Londino,Londn,London,London City,Londona,Londonas,Londoni,Londono,Londonu,Londra,Londres,Londrez,Londri,Londye,Londyn,Londýn,Lonn,Lontoo,Loundres,Luan GJon,Lunden,Lundra,Lundun,Lundunir,Lundúnir,Lung-dung,Lunnainn,Lunnin,Lunnon,Luân Đôn,Lùng-dŭng,Lākana,Lůndůn,Lọndọnu,Ranana,Rānana,The City,ilantan,landan,landana,leondeon,lndn,london,londoni,lun dui,lun dun,lwndwn,lxndxn,rondon,Łondra,Λονδίνο,Горад Лондан,Лондан,Лондон,Лондонъ,Лёндан,Լոնդոն,לאנדאן,לונדון,لندن,لوندون,لەندەن,ܠܘܢܕܘܢ,लंडन,लंदन,लण्डन,लन्डन्,লন্ডন,લંડન,ଲଣ୍ଡନ,இலண்டன்,లండన్,ಲಂಡನ್,ലണ്ടൻ,ලන්ඩන්,ลอนดอน,ລອນດອນ,ལོན་ཊོན།,လန်ဒန်မြို့,ლონდონი,ለንደን,ᎫᎴ ᏗᏍᎪᏂᎯᏱ,ロンドン,伦敦,倫敦,런던 GB ENG 51.50853 -0.12574 7556900 gcpvj0u6yjcm}
```

So you can get lat/lng from the ```GeobedCity``` struct real easily with: ```c.Latitude``` and ```c.Longitude```.

You'll notice some records are larger and contain many alternate names for the city. The free data sets come from Geonames and MaxMind. MaxMind has more but less details. Geonames has more details, but it only contains cities with populations of 1,000 people or greater (about 143,000 records).

If you looked up a major city, you'll likely have information such as population (```c.Population```).

You can reverse geocode as well.

```
c := g.ReverseGeocode(30.26715, -97.74306)
```

This would give you Austin, TX for example.

## Data Sets

The data sets are provided by [Geonames](http://download.geonames.org/export/dump) and [MaxMind](https://www.maxmind.com/en/worldcities). These are open source data sets. See their web sites for additional information.
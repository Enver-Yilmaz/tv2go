package naming

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hobeone/go-pcre"
	"github.com/hobeone/tv2go/quality"

	"github.com/gholt/brimtime"
	"github.com/golang/glog"
)

// NameRegex contains a compiled Regex and Name
type NameRegex struct {
	Name        string
	Regex       pcre.Regexp
	TestStrings []TestString
}

type TestString struct {
	String      string
	ShouldMatch bool
	MatchGroups map[string]string
}

var (
	mediaExtensions = []string{
		"avi", "mkv", "mpg", "mpeg", "wmv",
		"ogm", "mp4", "iso", "img", "divx",
		"m2ts", "m4v", "ts", "flv", "f4v",
		"mov", "rmvb", "vob", "dvr-ms", "wtv",
		"ogv", "3gp", "webm",
	}

	sampleRegex = regexp.MustCompile(`(?i)(^|[\W_])(sample\d*)[\W_]`)
	extrasRegex = regexp.MustCompile(`(?i)extras?$`)
)

// IsMediaExtension checks if the given string matches a known Media file
// extension.
func IsMediaExtension(extension string) bool {
	extension = strings.TrimLeft(extension, ".")
	extension = strings.ToLower(extension)
	for _, ext := range mediaExtensions {
		if ext == extension {
			return true
		}
	}
	return false
}

func stripExtension(fname string) string {
	extension := filepath.Ext(fname)
	return fname[0 : len(fname)-len(extension)]
}

// IsMediaFile checks if the given string is a media file
func IsMediaFile(filename string) bool {
	// ignore samples
	if sampleRegex.MatchString(filename) {
		return false
	}
	// ignore Mac resource fork files
	if strings.HasPrefix(filename, "._") {
		return false
	}

	extension := filepath.Ext(filename)
	name := stripExtension(filename)

	if extrasRegex.MatchString(name) {
		return false
	}

	return IsMediaExtension(extension)
}

/*
*
* Name Parser
 */

// ParseResult represents the result of parsing a filepath with a Regex.
type ParseResult struct {
	OriginalName           string
	SeriesName             string
	SeasonNumber           int64
	EpisodeNumbers         []int64
	ExtraInfo              string
	ReleaseGroup           string
	AirDate                time.Time
	AbsoluteEpisodeNumbers []int64
	Score                  int
	Quality                quality.Quality
	Version                string
	RegexUsed              string
}

func (r *ParseResult) FirstEpisode() int64 {
	if len(r.EpisodeNumbers) > 0 {
		return r.EpisodeNumbers[0]
	}
	if len(r.AbsoluteEpisodeNumbers) > 0 {
		return r.AbsoluteEpisodeNumbers[0]
	}
	return 0
}

type byScore []ParseResult

func (a byScore) Len() int           { return len(a) }
func (a byScore) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byScore) Less(i, j int) bool { return a[i].Score < a[j].Score }

type NameParser struct {
	Regexes []NameRegex
}

func NewNameParser(regexes []NameRegex) *NameParser {
	return &NameParser{
		Regexes: regexes,
	}
}

var knownMatches = []string{
	"series_name",
	"season_num",
	"ep_num",
	"ep_ab_num",
	"extra_ep_num",
	"extra_ab_ep_num",
	"extra_info",
	"release_group",
	"air_date",
	"series_num",
}

// Return named matches in a map
func regexNamedMatch(re *pcre.Regexp, str string) (map[string]string, bool) {
	m := re.MatcherString(str, 0)
	if !m.Matches() {
		return nil, false
	}

	result := make(map[string]string, len(knownMatches))
	for _, name := range knownMatches {
		extract, err := m.NamedString(name)
		if err == nil && extract != "" {
			result[name] = extract
		}
	}
	return result, true
}

func (np *NameParser) parseString(name string) (*ParseResult, error) {
	var matchResults []ParseResult
	for i, r := range np.Regexes {
		if matches, ok := regexNamedMatch(&r.Regex, name); ok {
			glog.Infof("Matched %s with regex %s", name, r.Name)
			pr := ParseResult{
				OriginalName: name,
				RegexUsed:    r.Name,
				Score:        0 - i,
			}

			if m, ok := matches["series_name"]; ok {
				pr.SeriesName = m
				pr.SeriesName = CleanSeriesName(pr.SeriesName)
				pr.Score++
			}
			if _, ok := matches["series_num"]; ok {
				pr.Score++
			}
			if m, ok := matches["season_num"]; ok {
				m = strings.TrimLeft(m, "0")
				sn, err := strconv.ParseInt(m, 10, 64)
				if err != nil {
					glog.Errorf("Error converting season_num '%s' to int from string: %s", m, pr.OriginalName)
					continue
				}
				pr.SeasonNumber = sn
				pr.Score++
			}
			if m, ok := matches["ep_num"]; ok {
				m = strings.TrimLeft(m, "0")
				// Maybe handle Roman numberals like SickRage?
				en, err := strconv.ParseInt(m, 10, 64)
				if err != nil {
					glog.Errorf("Error converting ep_num '%s' to int from string: %s", m, pr.OriginalName)
					continue
				}
				if extraEp, ok := matches["extra_ep_num"]; ok {
					m = strings.TrimLeft(m, "0")
					extraEpCvt, err := strconv.ParseInt(extraEp, 10, 64)
					if err != nil {
						glog.Errorf("Error converting extra_ep_num '%s' to int from string: %s", extraEp, pr.OriginalName)
					} else {
						pr.EpisodeNumbers = []int64{en, extraEpCvt}
					}
				} else {
					pr.EpisodeNumbers = []int64{en}
				}
			}
			if m, ok := matches["ep_ab_num"]; ok {
				en, err := strconv.ParseInt(m, 10, 64)
				if err != nil {
					glog.Errorf("Error converting ep_ab_num '%s' to int from string: %s", m, pr.OriginalName)
					continue
				}
				// Handle extra ab number
				pr.AbsoluteEpisodeNumbers = []int64{en}
			}
			if m, ok := matches["air_date"]; ok {
				year, month, day := brimtime.TranslateYMD(m, []string{"", "D", "MD", "YMD"})
				if year != 0 && month != 0 {
					pr.AirDate = time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
					pr.Score++
				} else {
					continue
				}
			}
			if _, ok := matches["extra_info"]; ok {
				// rando crap here
			}
			if m, ok := matches["release_group"]; ok {
				pr.ReleaseGroup = m
				pr.Score++
			}
			if m, ok := matches["version"]; ok {
				pr.Version = m
			}
			matchResults = append(matchResults, pr)
		}
	}

	// There's a whole mess of other logic that goes on in Sickbeard, but it's
	// super gross and we'll skip it for now:
	sort.Sort(sort.Reverse(byScore(matchResults)))
	if len(matchResults) > 0 {
		best := &matchResults[0]
		glog.Infof("Chose best match with regex %s, score %d", best.RegexUsed, best.Score)
		return &matchResults[0], nil
	}
	glog.Warningf("Couldn't match %s with any regex", name)
	return &ParseResult{}, fmt.Errorf("Couldn't parse string %s", name)
}

func (np *NameParser) Parse(name string) ParseResult {
	res, _ := np.parseString(name)
	q := quality.QualityFromName(name, false)
	if q == quality.UNKNOWN {
		glog.Warningf("Could't parse quality from '%s'", name)
	} else {
		glog.Infof("Found quality %s for '%s'", q.String(), name)
	}
	res.Quality = q
	return *res
}

// Parse tries to extract show and episode information from a file path. It
// considers both the filename and directory when trying to extract information
func (np *NameParser) ParseFile(name string) ParseResult {
	glog.Infof("Parsing string '%s' for show information", name)
	dirName, fileName := filepath.Split(name)
	fileName = stripExtension(fileName)
	dirNameBase := filepath.Base(dirName)

	fileNameResult, _ := np.parseString(fileName)
	dirNameResult, _ := np.parseString(dirNameBase)
	finalRes, _ := np.parseString(name)

	q := quality.QualityFromName(name, false)
	if q == quality.UNKNOWN {
		glog.Warningf("Could't parse quality from '%s'", name)
	} else {
		glog.Infof("Found quality %s for '%s'", q.String(), name)
	}
	combineResults(finalRes, fileNameResult, dirNameResult, "AirDate")
	combineResults(finalRes, fileNameResult, dirNameResult, "AbsoluteEpisodeNumbers")
	combineResults(finalRes, fileNameResult, dirNameResult, "SeasonNumber")
	combineResults(finalRes, fileNameResult, dirNameResult, "EpisodeNumbers")
	combineResults(finalRes, dirNameResult, fileNameResult, "SeriesName")
	combineResults(finalRes, dirNameResult, fileNameResult, "ExtraInfo")
	combineResults(finalRes, dirNameResult, fileNameResult, "ReleaseGroup")
	combineResults(finalRes, dirNameResult, fileNameResult, "Version")
	finalRes.Quality = q
	return *finalRes
}

// From src/pkg/encoding/json.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

func combineResults(finalRes, fileRes, dirRes *ParseResult, field string) error {
	r := reflect.Indirect(reflect.ValueOf(finalRes)).FieldByName(field)
	fVal := reflect.Indirect(reflect.ValueOf(fileRes)).FieldByName(field)
	dVal := reflect.Indirect(reflect.ValueOf(dirRes)).FieldByName(field)

	if !r.IsValid() || !fVal.IsValid() || !dVal.IsValid() {
		return fmt.Errorf("Invalid field name given: %s", field)
	}

	if r.CanSet() {
		if !isEmptyValue(fVal) {
			r.Set(fVal)
		} else {
			r.Set(dVal)
		}
	}

	return nil
}

// CleanSeriesName canonicalizes a series name extracted from a file name
func CleanSeriesName(name string) string {
	name = strings.Trim(name, " ")
	name = strings.Trim(name, ".")

	// Replace certain joining characters with a dash
	seps := regexp.MustCompile(`[\\/\*]`)
	name = seps.ReplaceAllString(name, "-")
	seps = regexp.MustCompile(`[:"<>|?]`)
	name = seps.ReplaceAllString(name, "")

	// Remove any double dashes caused by existing - in name
	name = strings.Replace(name, "--", "-", -1)
	return name
}

func SanitizeSceneName(name string) string {
	bad_chars := []string{",", ":", "(", ")", "!", "?", "'"}
	for _, str := range bad_chars {
		name = strings.Replace(name, str, "", -1)
	}
	name = strings.Replace(name, "- ", ".", -1)
	name = strings.Replace(name, " ", ".", -1)
	name = strings.Replace(name, "&", "and", -1)
	name = strings.Replace(name, "/", ".", -1)
	dotreplace := regexp.MustCompile(`\.\.*`)
	dotreplace.ReplaceAllString(name, ".")

	name = strings.TrimSuffix(name, ".")
	return name
}
func FullSanitizeSceneName(name string) string {
	name = SanitizeSceneName(name)
	badchars := regexp.MustCompile(`[. -]`)
	name = badchars.ReplaceAllString(name, " ")
	name = strings.ToLower(name)
	name = strings.TrimSpace(name)
	return name
}
